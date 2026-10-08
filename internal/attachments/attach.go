package attachments

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// UploadResult reports the successful upload and markdown operation counts.
type UploadResult struct {
	Uploaded          int
	AppendOperations  int
	ReplaceOperations int
}

// Document is Markdown for UploadAndAttachDocuments to attach files to.
type Document struct {
	// Markdown is the text whose references to attached files are rewritten.
	Markdown string
	// Dir is the directory of the file Markdown was read from, or "" when it
	// came from no file. It means what markdownDir means to UploadAndAttach.
	Dir string
}

// DocumentResult is what UploadAndAttachDocuments did to one Document.
//
// A document's attachments are the files it references, or every file when it
// is the only document.
type DocumentResult struct {
	// Markdown is the document with its references pointing at the URLs of its
	// uploaded attachments. An uploaded attachment with no reference it could
	// rewrite is appended. It is the document as given when it could not be
	// rewritten.
	Markdown string
	// Uploaded lists the document's attachments that reached the server, in
	// the order they were attached.
	Uploaded []UserAsset
	// AppendOperations and ReplaceOperations count how the uploads changed
	// Markdown, as they do in UploadResult.
	AppendOperations  int
	ReplaceOperations int
	// Err is the upload failure when it left one of the document's attachments
	// without a URL, because that file failed or was never tried, joined with
	// any failure to rewrite Markdown. It is nil when all of this document's
	// attachments uploaded and its Markdown was rewritten, even if an attachment
	// used only by another document failed.
	Err error
}

// UnreferencedError reports attached files that no document references.
// UploadAndAttachDocuments refuses them before anything uploads, unless there
// is exactly one document to append them to. Paths are as the user wrote them,
// in the order they were attached, so a caller can word the error for its own
// command.
type UnreferencedError struct {
	Paths []string
}

func (e *UnreferencedError) Error() string {
	return fmt.Sprintf("no document references %s", strings.Join(e.Paths, ", "))
}

// UploadAndAttach uploads assets in order and stops at the first failure. It
// points existing references at the URLs of successful uploads and appends only
// successful uploads the markdown did not reference. Assets after a failure
// are not attempted. It is UploadAndAttachDocuments with md as the only
// document.
//
// markdownDir is the directory of the file md was read from, such as a
// --body-file, or "" when md came from no file, such as --body or standard
// input. A relative reference resolves against markdownDir first, because a
// Markdown file's links are written relative to the file, and then against the
// working directory, so a reference written from there keeps matching.
//
// The result reports how many assets reached the server and how those successful
// uploads changed the markdown. A caller must write the returned markdown when
// Uploaded is above zero, even when the returned error is non-nil. An upload
// cannot be undone and there is no endpoint to delete one, so discarding that
// markdown would orphan the successful assets. An Uploaded count of zero means
// nothing was uploaded and nothing is lost by writing nothing.
//
// The markdown is returned unchanged when it could not be rewritten, so a
// caller that assigns the result in place never destroys what it was given.
func (u *Uploader) UploadAndAttach(ctx context.Context, md, markdownDir string, assets []UserAsset) (string, UploadResult, error) {
	results, err := u.UploadAndAttachDocuments(ctx, []Document{{Markdown: md, Dir: markdownDir}}, assets)
	if len(results) == 0 {
		return md, UploadResult{}, err
	}
	r := results[0]
	return r.Markdown, UploadResult{
		Uploaded:          len(r.Uploaded),
		AppendOperations:  r.AppendOperations,
		ReplaceOperations: r.ReplaceOperations,
	}, err
}

// UploadAndAttachDocuments attaches one set of files to several documents at
// once. It uploads each file once, in order, and stops at the first failure.
// Every document that references a file gets that file's URL. Each document's
// references resolve against its own Dir first and the working directory
// second.
//
// With one document, every file is its attachment, and the files it does not
// reference are appended, as UploadAndAttach does. With any other number, a
// file no document references has no document to be appended to, so it is
// refused with an *UnreferencedError.
//
// Every document is checked before the first upload, because an upload cannot
// be undone. A refusal returns no results, and nothing has uploaded. Otherwise
// there is one result per document, in the order given, and the error joins
// the upload failure with any document's failure to rewrite. A caller writes
// a document when its Err is nil or its Uploaded is not empty. It must write
// one with anything in Uploaded, even when its Err is set, for the reason
// UploadAndAttach gives. One whose Err is set and whose Uploaded is empty had
// every attachment fail or go untried, and loses nothing by not being written.
func (u *Uploader) UploadAndAttachDocuments(ctx context.Context, docs []Document, assets []UserAsset) ([]DocumentResult, error) {
	attachable, err := newAttachableDocuments(docs, assets)
	if err != nil {
		return nil, err
	}

	urls := make([]string, len(assets))
	var uploadErr error
	for i, a := range assets {
		assetURL, err := u.upload(ctx, a)
		if err != nil {
			// Stopping at the first failure. It makes recovery much simpler.
			uploadErr = err
			break
		}
		urls[i] = assetURL
	}

	failures := make([]error, 0, len(attachable)+1)
	failures = append(failures, uploadErr)
	results := make([]DocumentResult, len(attachable))
	for i, d := range attachable {
		result, rewriteErr := d.attach(assets, urls, uploadErr)
		results[i] = result
		failures = append(failures, rewriteErr)
	}
	return results, errors.Join(failures...)
}

// attachableDocument is one document scanned for the files it references,
// and which of the attached files are its attachments.
type attachableDocument struct {
	md attachableMarkdown
	// args is the slice md was scanned against, where the URLs of the
	// document's uploaded attachments are filled in. Each document has its
	// own, so a URL reaches only the documents the file is an attachment of.
	args []attachmentArg
	// isAttachment reports, for each attached file, whether it is one of the
	// document's attachments.
	isAttachment []bool
}

// newAttachableDocuments scans every document and refuses what no upload can
// fix, so that nothing uploads for a set of documents that cannot take it.
func newAttachableDocuments(docs []Document, assets []UserAsset) ([]attachableDocument, error) {
	out := make([]attachableDocument, len(docs))
	referenced := make([]bool, len(assets))
	for i, doc := range docs {
		args := newAttachmentArgs(assets)
		md, err := newAttachableMarkdown(doc.Markdown, doc.Dir, args)
		if err != nil {
			return nil, err
		}
		isAttachment := make([]bool, len(assets))
		for _, r := range md.refs {
			isAttachment[r.attachmentArg] = true
			referenced[r.attachmentArg] = true
		}
		out[i] = attachableDocument{md: md, args: args, isAttachment: isAttachment}
	}

	if len(docs) == 1 {
		// The only document is where every file goes, so the files it does
		// not reference are appended to it.
		for i := range out[0].isAttachment {
			out[0].isAttachment[i] = true
		}
		return out, nil
	}

	var unreferenced []string
	for i, a := range assets {
		if !referenced[i] {
			unreferenced = append(unreferenced, a.Path())
		}
	}
	if len(unreferenced) > 0 {
		return nil, &UnreferencedError{Paths: unreferenced}
	}
	return out, nil
}

// newAttachmentArgs describes each asset for the markdown half of the
// package, without a URL, since nothing has uploaded yet.
func newAttachmentArgs(assets []UserAsset) []attachmentArg {
	args := make([]attachmentArg, len(assets))
	for i, a := range assets {
		f := a.getAsset()
		args[i] = attachmentArg{Path: f.path, Alt: f.alt, RendersAsPlayer: a.rendersAsPlayer()}
	}
	return args
}

// attach points the document at the URLs of its attachments that uploaded,
// given urls, which holds an empty string for each file that did not. The
// error is a failure to rewrite the markdown, which is also in the result.
func (d attachableDocument) attach(assets []UserAsset, urls []string, uploadErr error) (DocumentResult, error) {
	result := DocumentResult{Markdown: d.md.markdown}
	for i, a := range assets {
		if !d.isAttachment[i] {
			continue
		}
		if urls[i] == "" {
			result.Err = uploadErr
			continue
		}
		d.args[i].URL = urls[i]
		result.Uploaded = append(result.Uploaded, a)
	}

	attachedMD, err := attachAssetsToMarkdown(d.md)
	if err != nil {
		result.Err = errors.Join(result.Err, err)
		return result, err
	}

	result.Markdown = appendUnreferenced(attachedMD, assets)
	result.AppendOperations = len(attachedMD.ToAppend)
	result.ReplaceOperations = attachedMD.ReplaceOperations
	return result, nil
}

// appendUnreferenced adds a paragraph for every attachment the author never
// referenced, in the order they were attached. Each one renders itself, since
// an image appends a markdown embed and a video appends a bare URL so that it
// plays, which is why this half does not live with the rewriting.
func appendUnreferenced(attachedMD attachedMarkdown, assets []UserAsset) string {
	urlByPath := make(map[string]string, len(attachedMD.ToAppend))
	for _, arg := range attachedMD.ToAppend {
		urlByPath[arg.Path] = arg.URL
	}

	out := attachedMD.Rewritten
	for _, a := range assets {
		url, ok := urlByPath[a.Path()]
		if !ok {
			continue
		}
		out = appendParagraph(out, a.markdown(url))
	}
	return out
}

// appendParagraph joins two pieces of markdown as separate paragraphs.
func appendParagraph(md, addition string) string {
	if addition == "" {
		return md
	}
	md = strings.TrimRight(md, " \t\r\n")
	if md == "" {
		return addition
	}
	return md + "\n\n" + addition
}
