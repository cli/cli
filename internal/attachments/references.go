package attachments

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// This file finds the places a body already points at an attached file, and
// repoints them. A file with no rewritten reference is reported in ToAppend and
// appended by attach.go, which knows that an image and a video append different
// markdown.
//
// The parts below run in that order: the two entry points, finding the
// references, working out which attached file each one names, locating its
// bytes, and making the edit.

// attachmentArg is one file named by an --attach argument, and the URL its
// contents were uploaded to. It is the command line half of this package; a
// markdown reference to it is an attachmentRef, and
// attachmentArgForDestination is where the two meet.
type attachmentArg struct {
	// Path is the local path as the flag was written. Matching is on the
	// absolute path.
	Path string
	// URL is where the contents now live. An argument with an empty URL did
	// not upload, so every reference to it is left as the author wrote it and
	// it is not reported for appending.
	URL string
	// Alt is the text that stands in for the file where the markdown supplies
	// none of its own: the link text of a video that degrades to a link, which
	// would otherwise have nothing to click.
	Alt string
	// RendersAsPlayer reports whether GitHub turns this file's URL into a
	// player rather than leaving it an image.
	RendersAsPlayer bool
}

// attachedMarkdown is the outcome of attaching, one field per flow. A file
// rewritten where the author referenced it contributes one replacement,
// regardless of how many references changed. A file that was not rewritten is
// left for the caller to append, because appending needs to know how an image
// and a video each render and this file does not.
type attachedMarkdown struct {
	Rewritten         string
	ToAppend          []attachmentArg
	ReplaceOperations int
}

// attachableMarkdown is markdown scanned for the files it references, held
// with the arguments it was scanned against. Attaching works from one of
// these, so the scan runs once and cannot afterwards be handed a set of
// arguments the scan never saw.
type attachableMarkdown struct {
	markdown string
	refs     []attachmentRef
	args     []attachmentArg
}

// newAttachableMarkdown scans markdown for the files it references, and
// refuses references that no asset URL can fix.
func newAttachableMarkdown(md string, attachmentArgs []attachmentArg) (attachableMarkdown, error) {
	if len(attachmentArgs) == 0 {
		return attachableMarkdown{markdown: md}, nil
	}
	refs, err := scanMarkdownRefs(md, attachmentArgs)
	if err != nil {
		return attachableMarkdown{}, err
	}
	if err := checkVideoReferenceImages(refs, attachmentArgs); err != nil {
		return attachableMarkdown{}, err
	}
	return attachableMarkdown{markdown: md, refs: refs, args: attachmentArgs}, nil
}

// attachAssetsToMarkdown points every rewritable markdown reference to an
// attached file at that file's asset URL. It reports one replacement per
// successfully rewritten attachment and which uploaded arguments remain to
// append.
//
//	![alt](./file) alone in a paragraph   image: swap the path   video: the bare URL, which plays
//	![alt](./file) anywhere else          image: swap the path   video: becomes [alt](URL)
//	[text](./file)                        swap the path, for images and video alike
//
// A reference-style link is rewritten in its definition instead, so every
// usage of that label follows from the one edit.
//
// A path inside a code fence or an inline code span is left alone.
func attachAssetsToMarkdown(v attachableMarkdown) (attachedMarkdown, error) {
	md := v.markdown
	if len(v.args) == 0 {
		return attachedMarkdown{Rewritten: md}, nil
	}

	src, refs, attachmentArgs := []byte(md), v.refs, v.args

	var edits []edit

	// Looping over the markdown references to produce a plan for edits.
	for _, r := range refs {
		arg := attachmentArgs[r.attachmentArg]

		// It's not been uploaded.
		if arg.URL == "" {
			continue
		}

		// Reference style links get rewritten at their definitions, not inline.
		if r.referenceStyle() {
			for _, def := range r.defs {
				dest, ok := linkReferenceDestination(src, def)
				// TODO: We skip the edit because a definition split across a blockquote
				// carries the ">" marker, and rewriting it would mangle the quote. The
				// fallback that appends unrewritten files operates per file. A rewritable
				// reference to the same file elsewhere suppresses the append fallback,
				// leaving this definition pointing at its original local path. We accept
				// this because the tool leaves the reference exactly as the author wrote
				// it, rewriting only what can be safely rewritten.
				if !ok {
					continue
				}
				edits = append(edits, edit{r.attachmentArg, dest, arg.URL})
			}
			continue
		}

		at := r.ranges
		playerEmbed := arg.RendersAsPlayer && r.isEmbed

		switch {
		case !playerEmbed:
			// Only the destination moves, so alt text, titles, and formatting
			// inside the label survive.
			edits = append(edits, edit{r.attachmentArg, at.dest, arg.URL})

		case standsAlone(src, r.block, at.node):
			// A player only renders from a bare URL alone in a paragraph, so
			// the whole node goes and any alt text is dropped.
			edits = append(edits, edit{r.attachmentArg, at.node, arg.URL})

		default:
			// Degrade to a link: only the "!" and the destination move, so a
			// reference nested in the alt text is still rewritten in place.
			//
			// A literal "!" before the embed's own would pair with the "["
			// left behind and re-form an embed, so it is absorbed into the
			// same edit and escaped. One already escaped is left alone, since
			// escaping it twice emits a literal backslash and re-forms the
			// embed anyway.
			bang := byteRange{at.node.start, at.node.start + 1}
			drop := ""
			if bang.start > 0 && src[bang.start-1] == '!' && !isEscaped(src, bang.start-1) {
				bang.start--
				drop = `\!`
			}
			edits = append(edits, edit{r.attachmentArg, bang, drop})

			if at.label.start == at.label.stop {
				// The author wrote no alt text, and a video has none to
				// inherit, so the link would have nothing to click. The name
				// newAsset fell back to stands in. It is escaped because a
				// name may contain the brackets that end a label.
				edits = append(edits, edit{r.attachmentArg, at.label, escapeAlt(arg.Alt)})
			}
			edits = append(edits, edit{r.attachmentArg, at.dest, arg.URL})
		}
	}

	out, written := applyEdits(md, edits)

	var unreferenced []attachmentArg
	replaced := 0
	for i, a := range attachmentArgs {
		if a.URL == "" {
			continue
		}
		if written[i] {
			replaced++
		} else {
			unreferenced = append(unreferenced, a)
		}
	}
	return attachedMarkdown{
		Rewritten:         out,
		ToAppend:          unreferenced,
		ReplaceOperations: replaced,
	}, nil
}

// checkVideoReferenceImages returns an error naming every video the markdown
// embeds through a reference definition. Rewriting one would produce an image
// embed of a video, which renders as a broken image.
func checkVideoReferenceImages(refs []attachmentRef, attachmentArgs []attachmentArg) error {
	var paths []string
	seen := map[int]bool{}
	for _, r := range refs {
		if !r.referenceStyle() || !r.isEmbed {
			continue
		}
		if !attachmentArgs[r.attachmentArg].RendersAsPlayer || seen[r.attachmentArg] {
			continue
		}
		seen[r.attachmentArg] = true
		paths = append(paths, attachmentArgs[r.attachmentArg].Path)
	}
	if len(paths) == 0 {
		return nil
	}
	return fmt.Errorf("cannot embed a video as a reference-style image: %s", strings.Join(paths, ", "))
}

// attachmentRef is one markdown reference to an attached file: the markdown
// half of this package, paired to an argument by attachmentArgForDestination.
//
// It is written either inline, as [text](./file), or through a reference
// definition. An inline one carries the ranges and block that locate it in the
// source; a reference-style one carries the definitions holding its
// destination, and is recognised by defs being non-empty.
type attachmentRef struct {
	// attachmentArg indexes the attached file this reference names.
	attachmentArg int
	// isEmbed reports whether it was written with a leading "!".
	isEmbed bool

	ranges refRanges
	block  ast.Node

	defs []*ast.LinkReferenceDefinition
}

// referenceStyle reports whether the destination lives in a definition rather
// than beside the reference.
func (r attachmentRef) referenceStyle() bool { return len(r.defs) > 0 }

// scanMarkdownRefs finds every place the markdown names one of the attached
// files.
func scanMarkdownRefs(md string, attachmentArgs []attachmentArg) ([]attachmentRef, error) {
	byPath, err := attachmentArgsByPath(attachmentArgs)
	if err != nil {
		return nil, err
	}

	src := []byte(md)
	doc := goldmark.New().Parser().Parse(text.NewReader(src))
	defs := linkReferenceDefinitions(doc, len(src))

	var refs []attachmentRef

	err = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		dest, isEmbed, ok := linkDestination(n)
		if !ok {
			return ast.WalkContinue, nil
		}
		idx, ok := attachmentArgForDestination(dest, byPath)
		if !ok {
			return ast.WalkContinue, nil
		}

		// The nearest block ancestor is what carries position information.
		var block ast.Node
		for p := n.Parent(); p != nil; p = p.Parent() {
			if p.Type() == ast.TypeBlock {
				block = p
				break
			}
		}
		_, hi, ok := blockRange(block, len(src))
		if !ok {
			return ast.WalkContinue, nil
		}

		if ranges, ok := inlineRanges(src, n, hi); ok {
			refs = append(refs, attachmentRef{attachmentArg: idx, isEmbed: isEmbed, ranges: ranges, block: block})
			return ast.WalkContinue, nil
		}

		// No destination beside the reference means a reference-style link. Every
		// definition carrying this destination is rewritten, since a node
		// records no label to tell them apart and a spare definition renders
		// nothing.
		if matches := defs[comparableDestination(dest)]; len(matches) > 0 {
			refs = append(refs, attachmentRef{attachmentArg: idx, isEmbed: isEmbed, defs: matches})
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func linkDestination(n ast.Node) (dest string, isEmbed bool, ok bool) {
	switch v := n.(type) {
	case *ast.Image:
		return string(v.Destination), true, true
	case *ast.Link:
		return string(v.Destination), false, true
	}
	return "", false, false
}

// linkReferenceDefinitions indexes every one in the document by its
// destination. One inside a code fence or a code span is not parsed as a
// definition at all, so both are excluded for free.
func linkReferenceDefinitions(doc ast.Node, size int) map[string][]*ast.LinkReferenceDefinition {
	out := map[string][]*ast.LinkReferenceDefinition{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		def, ok := n.(*ast.LinkReferenceDefinition)
		if !ok {
			return ast.WalkContinue, nil
		}
		if _, _, ok := blockRange(def, size); !ok {
			return ast.WalkContinue, nil
		}
		key := comparableDestination(string(def.Destination))
		out[key] = append(out[key], def)
		return ast.WalkContinue, nil
	})
	return out
}

// attachmentArgsByPath indexes the attached files by absolute path, ready for
// attachmentArgForDestination to look one up. An argument with no URL is
// indexed too, so validation can run before anything has uploaded.
func attachmentArgsByPath(attachmentArgs []attachmentArg) (map[string]int, error) {
	byPath := make(map[string]int, len(attachmentArgs))
	for i, a := range attachmentArgs {
		if a.Path == "" {
			continue
		}
		abs, err := filepath.Abs(a.Path)
		if err != nil {
			return nil, err
		}
		if _, dup := byPath[abs]; dup {
			continue
		}
		byPath[abs] = i
	}
	return byPath, nil
}

// attachmentArgForDestination is where the two halves of this package meet: it
// takes a markdown destination and answers which attached file, if any, it
// names.
//
// Only a local path can name one, and markdown offers several spellings for
// the same path, so each is resolved to an absolute path and looked up.
func attachmentArgForDestination(dest string, byPath map[string]int) (int, bool) {
	if dest == "" || strings.HasPrefix(dest, "#") || isRemoteDestination(dest) {
		return 0, false
	}
	for _, path := range candidatePaths(dest) {
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if i, ok := byPath[abs]; ok {
			return i, true
		}
	}
	return 0, false
}

// isRemoteDestination reports whether a destination addresses somewhere other
// than the filesystem, so that an attached file is never confused with a URI.
//
// A one letter scheme is a Windows volume rather than a scheme, which keeps
// "C:\pictures\login.png" a path. A host with no scheme is the protocol
// relative form, which inherits the scheme of the page it sits on. Anything too
// malformed to parse is left to the path lookup, which will not match it.
func isRemoteDestination(dest string) bool {
	u, err := url.Parse(dest)
	if err != nil {
		return false
	}
	return len(u.Scheme) > 1 || u.Host != ""
}

// candidatePaths returns the ways a destination could name a file on disk.
// goldmark reports the destination alone, without the angle brackets or the
// whitespace that separated it from the rest of the link, so what arrives is
// already the name. Backslash escapes and percent encoding survive, and either
// can stand in for a character legal in a filename. A space must be written as
// "<./my file.png>" or "./my%20file.png" to parse at all, which is also why a
// space inside the brackets belongs to the name and is kept.
func candidatePaths(dest string) []string {
	out := []string{dest}
	if unescaped := unescapePunctuation(dest); unescaped != dest {
		out = append(out, unescaped)
	}
	for _, s := range append([]string{}, out...) {
		if decoded, err := url.PathUnescape(s); err == nil && decoded != s {
			out = append(out, decoded)
		}
	}
	return out
}

// comparableDestination reduces a destination goldmark reported to the single
// form used to decide whether a run of source is the node it came from. Percent
// encoding is left as written, since goldmark reports it unchanged and both
// sides of every comparison therefore carry it.
func comparableDestination(dest string) string {
	return unescapePunctuation(dest)
}

// comparableSource reduces a destination read straight from the source to that
// same form. It still carries the syntax goldmark had already removed: the
// whitespace separating the destination from the rest of the link, and the
// angle brackets that let a destination hold a space. Whitespace inside those
// brackets is part of the name, so only the whitespace outside them goes.
func comparableSource(raw string) string {
	return comparableDestination(trimAngles(strings.TrimSpace(raw)))
}

func trimAngles(s string) string {
	if len(s) >= 2 && s[0] == '<' && s[len(s)-1] == '>' {
		return s[1 : len(s)-1]
	}
	return s
}

// unescapePunctuation drops the backslash from every backslash-escaped ASCII
// punctuation character, which is how markdown spells a literal one.
func unescapePunctuation(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	const punct = `!"#$%&'()*+,-./:;<=>?@[\]^_` + "`" + `{|}~`
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) && strings.IndexByte(punct, s[i+1]) >= 0 {
			i++
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

type byteRange struct {
	start, stop int
}

// refRanges locates the raw source that produced an inline link or image.
type refRanges struct {
	// node covers the whole thing, from the "!" or "[" through the ")".
	node byteRange
	// label covers what sits between the brackets.
	label byteRange
	// dest covers the destination, angle brackets included.
	dest byteRange
}

// blockRange returns the byte range a block covers in the source.
func blockRange(b ast.Node, size int) (int, int, bool) {
	if b == nil {
		return 0, 0, false
	}
	lines := b.Lines()
	if lines == nil || lines.Len() == 0 {
		return 0, 0, false
	}
	lo, hi := lines.At(0).Start, lines.At(lines.Len()-1).Stop
	if lo < 0 || hi > size || lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

// inlineRanges locates the source of an inline link or image. goldmark reports
// where every inline node starts, so the only thing left to find is where it
// ends, inside a node goldmark has already accepted.
//
// A reference-style link carries no destination beside it and reports false,
// leaving it to the definitions. hi bounds the search to the block.
func inlineRanges(src []byte, n ast.Node, hi int) (refRanges, bool) {
	start := n.Pos()
	if start < 0 || start >= hi {
		return refRanges{}, false
	}
	open := start
	if src[open] == '!' {
		open++
	}
	if open >= hi || src[open] != '[' {
		return refRanges{}, false
	}
	close, ok := closingBracket(src, open, hi, literalText(n))
	if !ok || close+1 >= hi || src[close+1] != '(' {
		return refRanges{}, false
	}
	dest := inlineDestination(src, close+2, hi)
	stop := skipSpace(src, dest.stop, hi)
	if stop < hi && src[stop] != ')' {
		if end, ok := scanTitle(src, stop, hi); ok {
			stop = skipSpace(src, end, hi)
		}
	}
	if stop >= hi || src[stop] != ')' {
		return refRanges{}, false
	}
	return refRanges{
		node:  byteRange{start, stop + 1},
		label: byteRange{open + 1, close},
		dest:  dest,
	}, true
}

// closingBracket finds the "]" that ends a label opened at open, counting the
// brackets of anything nested inside it and ignoring those in literal.
func closingBracket(src []byte, open, hi int, literal []byteRange) (int, bool) {
	depth := 0
	for i := open; i < hi; i++ {
		if isEscaped(src, i) || within(literal, i) {
			continue
		}
		switch src[i] {
		case '[':
			depth++
		case ']':
			if depth--; depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// literalText reports the byte ranges inside n that a code span quotes, where a
// bracket is a character rather than markdown. goldmark parsed them, so their
// text nodes already say where they are.
func literalText(n ast.Node) []byteRange {
	var out []byteRange
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*ast.CodeSpan); !ok {
			out = append(out, literalText(c)...)
			continue
		}
		for t := c.FirstChild(); t != nil; t = t.NextSibling() {
			if text, ok := t.(*ast.Text); ok {
				out = append(out, byteRange{text.Segment.Start, text.Segment.Stop})
			}
		}
	}
	return out
}

// within reports whether i falls inside any of ranges.
func within(ranges []byteRange, i int) bool {
	for _, s := range ranges {
		if i >= s.start && i < s.stop {
			return true
		}
	}
	return false
}

// inlineDestination finds the destination written beside a reference, as the
// "./f.png" of [text](./f.png).
func inlineDestination(src []byte, i, hi int) byteRange {
	i = skipSpace(src, i, hi)
	if i < hi && src[i] == '<' {
		if stop, ok := scanAngleDest(src, i, hi); ok {
			return byteRange{i, stop}
		}
	}
	return byteRange{i, scanBareDest(src, i, hi)}
}

// scanBareDest reads an unbracketed destination, which ends at whitespace or
// at the ")" closing the link. Parentheses inside the path are part of it as
// long as they balance.
func scanBareDest(src []byte, i, hi int) int {
	depth := 0
	for i < hi {
		switch c := src[i]; {
		case c == '\\':
			if i+1 < hi {
				i++
			}
		case isSpace(c):
			return i
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				return i
			}
			depth--
		}
		i++
	}
	return i
}

// scanTitle skips the optional title following a destination and returns the
// index just past it. i may point at anything: a byte that opens no title
// leaves the index untouched. A title is delimited by double quotes, single
// quotes, or parentheses.
func scanTitle(src []byte, i, hi int) (int, bool) {
	var closer byte
	switch src[i] {
	case '"', '\'':
		closer = src[i]
	case '(':
		closer = ')'
	default:
		return i, true
	}
	i++
	for i < hi && src[i] != closer {
		if src[i] == '\\' && i+1 < hi {
			i++
		}
		i++
	}
	if i >= hi {
		return 0, false
	}
	return i + 1, true
}

// scanAngleDest reads a "<...>" destination beginning at the "<", returning the
// index just past the closing ">".
func scanAngleDest(src []byte, i, hi int) (int, bool) {
	i++
	for i < hi && src[i] != '>' && src[i] != '\n' {
		if src[i] == '\\' && i+1 < hi {
			i++
		}
		i++
	}
	if i >= hi || src[i] != '>' {
		return 0, false
	}
	return i + 1, true
}

// linkReferenceDestination finds the destination in a link reference
// definition, as the "./f.png" of [ref]: ./f.png "a title", so that rewriting
// it leaves the label and the title as the author wrote them.
//
// It reads the destination out of the source and refuses if that disagrees
// with what goldmark parsed, which means the line is shaped in some way this
// does not understand and is safer left alone.
func linkReferenceDestination(src []byte, def *ast.LinkReferenceDefinition) (byteRange, bool) {
	lo, hi, ok := blockRange(def, len(src))
	if !ok {
		return byteRange{}, false
	}

	// Past the label, which ends at the first unescaped "]:".
	colon := -1
	for off, c := range src[lo:hi] {
		i := lo + off
		if c == ']' && i+1 < hi && src[i+1] == ':' && !isEscaped(src, i) {
			colon = i + 1
			break
		}
	}
	if colon < 0 {
		return byteRange{}, false
	}

	start := skipSpace(src, colon+1, hi)
	if start >= hi {
		return byteRange{}, false
	}
	stop := start
	if src[stop] == '<' {
		end, ok := scanAngleDest(src, stop, hi)
		if !ok {
			return byteRange{}, false
		}
		stop = end
	} else {
		stop = scanBareDest(src, stop, hi)
	}

	// A definition continued onto the next line of a blockquote is the case
	// that reaches here: the block range still carries the ">" prefix of the
	// continuation, so the destination read from the source is that marker
	// rather than the path, and rewriting it would eat the blockquote.
	if comparableSource(string(src[start:stop])) != comparableDestination(string(def.Destination)) {
		return byteRange{}, false
	}
	return byteRange{start: start, stop: stop}, true
}

// isEscaped reports whether the byte at i is preceded by an odd number of
// backslashes, which is what makes it a literal rather than markup.
func isEscaped(src []byte, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && src[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

func skipSpace(src []byte, i, hi int) int {
	for i < hi && isSpace(src[i]) {
		i++
	}
	return i
}

// isSpace reports whether c is ASCII whitespace, which is what ends an
// unbracketed markdown destination.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// edit describes a replacement of a byte range of the markdown.
type edit struct {
	a    int
	at   byteRange
	text string
}

// applyEdits rewrites the markdown back to front, so that an earlier edit cannot
// shift a later one, and reports which arguments were actually written.
//
// Edits overlap when a video embed alone in its paragraph is replaced by a
// bare URL and its alt text carries a reference of its own. The wider edit
// wins, and the dropped one's argument is reported unwritten so the caller
// appends it.
func applyEdits(md string, edits []edit) (string, map[int]bool) {
	written := map[int]bool{}
	if len(edits) == 0 {
		return md, written
	}

	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].at.start != edits[j].at.start {
			return edits[i].at.start < edits[j].at.start
		}
		return edits[i].at.stop > edits[j].at.stop
	})

	var kept []edit
	end := 0
	for _, e := range edits {
		if e.at.start < end || e.at.start > e.at.stop || e.at.stop > len(md) {
			continue
		}
		kept = append(kept, e)
		written[e.a] = true
		end = e.at.stop
	}

	out := md
	for _, e := range slices.Backward(kept) {
		out = out[:e.at.start] + e.text + out[e.at.stop:]
	}
	return out, written
}

// standsAlone reports whether replacing a node with a bare URL will render as
// a player.
//
// GitHub promotes a bare asset URL to a video exactly when it is the whole
// content of a paragraph, wherever that paragraph sits: a blockquote or a
// loose list item qualifies, a URL alone on a source line inside a wrapped
// paragraph does not. A tight list item holds no paragraph, only a text
// block, which is why a lone reference there stays a link. goldmark draws the
// same distinction, so testing for a paragraph is the whole rule.
func standsAlone(src []byte, block ast.Node, node byteRange) bool {
	if _, ok := block.(*ast.Paragraph); !ok {
		return false
	}
	lo, hi, ok := blockRange(block, len(src))
	// The node always falls inside the block that produced it, so the range
	// test here only keeps the slices below in bounds.
	if !ok || node.start < lo || node.stop > hi {
		return false
	}
	for _, c := range src[lo:node.start] {
		if !isSpace(c) {
			return false
		}
	}
	for _, c := range src[node.stop:hi] {
		if !isSpace(c) {
			return false
		}
	}
	return true
}

// isSingleImage reports whether src is one image pointing at wantURL and
// nothing besides. Asset validation uses it to confirm that escaped alt text
// cannot restructure the markdown it is placed in.
func isSingleImage(src, wantURL string) bool {
	doc := goldmark.New().Parser().Parse(text.NewReader([]byte(src)))

	block := doc.FirstChild()
	if block == nil || block.NextSibling() != nil {
		return false
	}
	inline := block.FirstChild()
	if inline == nil || inline.NextSibling() != nil {
		return false
	}
	image, ok := inline.(*ast.Image)
	return ok && string(image.Destination) == wantURL
}
