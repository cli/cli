package attachments

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/internal/text"
)

// maxImageBytes is the largest image gh uploads.
const maxImageBytes int64 = 10 * 1024 * 1024

// maxVideoBytes is the largest video gh uploads. The real limit depends on the
// account plan, which gh cannot know before the request, so this is the
// generous bound and the server refuses the rest.
const maxVideoBytes int64 = 100 * 1024 * 1024

// contentTypes maps every accepted extension to the content type the endpoint
// expects, in the order gh lists them back to the user.
var contentTypes = []struct {
	ext         string
	contentType string
}{
	{".png", "image/png"},
	{".jpg", "image/jpeg"},
	{".jpeg", "image/jpeg"},
	{".gif", "image/gif"},
	{".webp", "image/webp"},
	{".svg", "image/svg+xml"},
	{".mp4", "video/mp4"},
	{".mov", "video/quicktime"},
	{".webm", "video/webm"},
}

// UserAsset is one validated local file, and everything needed to construct the
// markdown to reference it once it is uploaded.
type UserAsset interface {
	// Path is the file as the user wrote it, so a caller naming it back names
	// what they typed.
	Path() string

	rendersAsPlayer() bool
	getAsset() asset
	markdown(assetURL string) string
}

type asset struct {
	path        string
	info        fs.FileInfo
	alt         string
	contentType string
}

// imageAsset renders as a markdown image.
type imageAsset struct{ asset }

func (a *imageAsset) Path() string    { return a.asset.path }
func (a *imageAsset) getAsset() asset { return a.asset }

func (*imageAsset) rendersAsPlayer() bool { return false }

// newImageAsset applies what only holds for an image: the author may write alt
// text, and the fallback mirrors the web uploader by stripping the extension
// and replacing the remaining dots with spaces.
func newImageAsset(f asset, alt string) (UserAsset, error) {
	a := &imageAsset{f}
	if err := checkMaxSize(a.asset, maxImageBytes, "images"); err != nil {
		return nil, err
	}

	if alt == "" {
		base := filepath.Base(a.path)
		alt = strings.ReplaceAll(strings.TrimSuffix(base, filepath.Ext(base)), ".", " ")
	}
	a.alt = alt

	if err := checkAltStaysInside(a); err != nil {
		return nil, err
	}

	return a, nil
}

func (a *imageAsset) markdown(assetURL string) string {
	return fmt.Sprintf("![%s](%s)", escapeAlt(a.alt), assetURL)
}

// videoAsset renders as a player, which markdown has no syntax for: GitHub
// promotes a bare URL that is the whole content of a paragraph.
type videoAsset struct{ asset }

func (a *videoAsset) Path() string    { return a.asset.path }
func (a *videoAsset) getAsset() asset { return a.asset }

func (*videoAsset) rendersAsPlayer() bool { return true }

// newVideoAsset applies what only holds for a video: a player has no alt
// attribute to fill, so the author cannot supply one, and the name that stands
// in keeps its extension because it goes where a filename would in a link.
func newVideoAsset(f asset, alt string) (UserAsset, error) {
	a := &videoAsset{f}
	if err := checkMaxSize(a.asset, maxVideoBytes, "videos"); err != nil {
		return nil, err
	}

	if alt != "" {
		return nil, errors.New("cannot set alt text on video")
	}
	a.alt = filepath.Base(a.path)

	return a, nil
}

func (*videoAsset) markdown(assetURL string) string { return assetURL }

// newAsset validates one attached file.
func newAsset(path, alt string) (UserAsset, error) {
	fi, err := os.Stat(path)
	if err != nil {
		// Drop the syscall name so the message names the file, not the
		// operation gh performed on it.
		if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
			return nil, fmt.Errorf("%s: %w", path, pathErr.Err)
		}
		return nil, err
	}

	if fi.IsDir() {
		return nil, fmt.Errorf("%s is a directory", path)
	}

	// Stat succeeds on a named pipe and Read then blocks forever.
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}

	// Nothing downstream objects to zero bytes, so an empty file uploads and
	// renders broken.
	if fi.Size() == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}

	contentType, err := supportedContentType(path)
	if err != nil {
		return nil, err
	}

	f := asset{path: path, info: fi, contentType: contentType}

	if strings.HasPrefix(contentType, "video/") {
		return newVideoAsset(f, alt)
	}
	return newImageAsset(f, alt)
}

// checkMaxSize rejects a file over the limit for its kind. The limit is
// inclusive, so the message says at most rather than under.
func checkMaxSize(a asset, maxBytes int64, kind string) error {
	if a.info.Size() <= maxBytes {
		return nil
	}
	return fmt.Errorf("%s: %s must be at most %s", a.path, kind, text.FormatSize(maxBytes))
}

// supportedContentType maps a file extension to the content type the endpoint
// expects. The extension is all gh checks, since the endpoint accepts
// mislabeled bytes anyway.
func supportedContentType(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	for _, t := range contentTypes {
		if t.ext == ext {
			return t.contentType, nil
		}
	}

	supported := make([]string, len(contentTypes))
	for i, t := range contentTypes {
		supported[i] = strings.TrimPrefix(t.ext, ".")
	}
	return "", fmt.Errorf("%s is not a supported file type (supported: %s)", path, strings.Join(supported, ", "))
}

// altEscaper neutralizes the characters that let alt text break out of the
// image syntax. Without it, alt text containing `](url)` closes the image early
// and points it somewhere the author did not choose.
var altEscaper = strings.NewReplacer(
	`\`, `\\`,
	`[`, `\[`,
	`]`, `\]`,
	"\n", " ",
	"\r", " ",
)

func escapeAlt(s string) string {
	return altEscaper.Replace(s)
}

// checkAltStaysInside renders the image this asset will produce and parses it
// back, so alt text that escapeAlt failed to neutralize is refused before the
// upload rather than discovered after it. An upload cannot be undone, which is
// why the check runs at validation.
//
// The URL is a stand-in, since the real one does not exist until the asset has
// uploaded. Only the alt text varies, so a stand-in proves the same thing.
func checkAltStaysInside(a *imageAsset) error {
	const probeURL = "https://example.invalid/probe"
	if !isSingleImage(a.markdown(probeURL), probeURL) {
		return fmt.Errorf("%s: alt text cannot be rendered safely", a.path)
	}
	return nil
}
