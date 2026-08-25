package attachments

import (
	"context"
	"errors"
	"strings"
)

// UploadAndAttach uploads assets in order and stops at the first failure. It
// points existing references at the URLs of successful uploads and appends only
// successful uploads the markdown did not reference. Assets after a failure
// are not attempted.
//
// The returned count reports how many assets reached the server. A caller must
// write the returned markdown when that count is above zero, even when the
// returned error is non-nil. An upload cannot be undone and there is no endpoint
// to delete one, so discarding that markdown would orphan the successful assets.
// A count of zero means nothing was uploaded and nothing is lost by writing
// nothing.
//
// The markdown is returned unchanged when it could not be rewritten, so a
// caller that assigns the result in place never destroys what it was given.
func (u *Uploader) UploadAndAttach(ctx context.Context, md string, assets []UserAsset) (string, int, error) {
	args := make([]attachmentArg, len(assets))
	for i, a := range assets {
		f := a.getAsset()
		args[i] = attachmentArg{Path: f.path, Alt: f.alt, RendersAsPlayer: a.rendersAsPlayer()}
	}

	attachableMD, err := newAttachableMarkdown(md, args)
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
		args[i].URL = assetURL
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
	if addition == "" {
		return md
	}
	md = strings.TrimRight(md, " \t\r\n")
	if md == "" {
		return addition
	}
	return md + "\n\n" + addition
}
