package attachments

import (
	"context"
	"errors"
	"strings"
)

// UploadAndAttach uploads every asset, points the references the markdown
// already wrote at their asset URLs, and appends the rest. It reports how many
// assets reached the server.
//
// That count is the only reason to write markdown alongside an error. An upload
// cannot be undone and there is no endpoint to delete one, so markdown that does
// not reference an uploaded asset orphans it for good. A count of zero means
// nothing was uploaded and nothing is lost by writing nothing.
//
// The markdown is returned unchanged when it could not be rewritten, so a
// caller that assigns the result in place never destroys what it was given.
func (u *Uploader) UploadAndAttach(ctx context.Context, md string, assets []UserAsset) (string, int, error) {
	refs := make([]attachmentArg, len(assets))
	for i, a := range assets {
		f := a.file()
		refs[i] = attachmentArg{Path: f.path, Alt: f.alt, RendersAsPlayer: a.rendersAsPlayer()}
	}

	attachableMD, err := newAttachableMarkdown(md, refs)
	if err != nil {
		return md, 0, err
	}

	var failures []error
	uploaded := 0
	for i, a := range assets {
		assetURL, err := u.upload(ctx, a)
		if err != nil {
			// Stopping at the first failure. It makes recovery much simpler.
			failures = append(failures, err)
			break
		}
		refs[i].URL = assetURL
		uploaded++
	}

	attachedMD, err := attachAssetsToMarkdown(attachableMD)
	if err != nil {
		failures = append(failures, err)
		return md, uploaded, errors.Join(failures...)
	}

	return appendUnreferenced(attachedMD, assets), uploaded, errors.Join(failures...)
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
	md = strings.TrimRight(md, " \t\r\n")
	if md == "" {
		return addition
	}
	if addition == "" {
		return md
	}
	return md + "\n\n" + addition
}
