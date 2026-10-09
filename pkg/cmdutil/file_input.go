package cmdutil

import (
	"io"
	"os"
	"path/filepath"
)

func ReadFile(filename string, stdin io.ReadCloser) ([]byte, error) {
	if filename == "-" {
		b, err := io.ReadAll(stdin)
		_ = stdin.Close()
		return b, err
	}

	return os.ReadFile(filename)
}

// ReadMarkdownBodyFile reads Markdown from path and returns its base directory
// for resolving local attachment references. Standard input has no base
// directory.
func ReadMarkdownBodyFile(path string, stdin io.ReadCloser) (content, dir string, err error) {
	b, err := ReadFile(path, stdin)
	if err != nil {
		return "", "", err
	}
	if path != "-" {
		dir = filepath.Dir(path)
	}
	return string(b), dir, nil
}
