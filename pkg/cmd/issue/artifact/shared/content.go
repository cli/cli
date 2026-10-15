package shared

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	gistShared "github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

// ReadContent reads an artifact's new content from the file at path,
// following a symlink, or from stdin when path is "-". It also returns the
// directory the content's references to attached files resolve against,
// which is empty for stdin, as cmdutil.ReadMarkdownBodyFile gives it.
// Artifact content must be text, like a gist's, so binary content is refused,
// and so is content that is empty or only whitespace. Callers read every file
// before any request, so these refusals fail the whole command. The error
// doesn't name path, so a caller can say which artifact failed in its own
// words.
func ReadContent(path string, stdin io.ReadCloser) (content, dir string, err error) {
	what := "file"
	if path == "-" {
		what = "input"
	}

	content, dir, err = cmdutil.ReadMarkdownBodyFile(path, stdin)
	if err != nil {
		if pathErr, ok := errors.AsType[*fs.PathError](err); ok && pathErr.Path == path {
			return "", "", pathErr.Err
		}
		return "", "", err
	}

	if strings.TrimSpace(content) == "" {
		return "", "", fmt.Errorf("%s is empty", what)
	}
	if gistShared.IsBinaryContents([]byte(content)) {
		return "", "", fmt.Errorf("binary %s not supported", what)
	}
	return content, dir, nil
}

// IsShortcutFile reports whether path is an Internet Shortcut file, which
// artifact commands read as a link. Like Windows, it goes by the .url
// extension, in any case.
func IsShortcutFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".url")
}

// ErrNoShortcutSection is ShortcutURL's error for content with no
// [InternetShortcut] section, such as a bare URL, so a caller can add a hint.
var ErrNoShortcutSection = errors.New("no [InternetShortcut] section")

// ShortcutURL returns the URL an Internet Shortcut file's content links to,
// read the way Windows reads it: the first URL= in the [InternetShortcut]
// section, ignoring case, comments, other keys and other sections. Content
// without that section and key, or whose URL isn't an http(s) URL as HTTPURL
// defines it, is an error rather than a guess.
func ShortcutURL(content string) (string, error) {
	// Windows saves some shortcuts with a UTF-8 byte order mark.
	content = strings.TrimPrefix(content, "\ufeff")

	inSection, sawSection := false, false
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, ";") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "["); ok {
			name, _, _ := strings.Cut(rest, "]")
			inSection = strings.EqualFold(strings.TrimSpace(name), "InternetShortcut")
			sawSection = sawSection || inSection
			continue
		}
		if !inSection {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "URL") {
			continue
		}
		u, ok := HTTPURL(value)
		if !ok {
			return "", errors.New("the shortcut doesn't link to an http(s) URL")
		}
		return u, nil
	}

	if !sawSection {
		return "", ErrNoShortcutSection
	}
	return "", errors.New("no URL in the [InternetShortcut] section")
}
