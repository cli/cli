package browser

import (
	"io"

	ghBrowser "github.com/cli/go-gh/v2/pkg/browser"
)

type Browser interface {
	Browse(string) error
}

func New(launcher string, stdout, stderr io.Writer) Browser {
	b := ghBrowser.New(launcher, stdout, stderr)
	return b
}
