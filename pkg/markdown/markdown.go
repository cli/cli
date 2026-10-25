package markdown

import (
	"os"
	"strconv"

	"charm.land/glamour/v2"
	ghMarkdown "github.com/cli/go-gh/v2/pkg/markdown"
)

func WithoutIndentation() glamour.TermRendererOption {
	return ghMarkdown.WithoutIndentation()
}

func WithWrap(w int) glamour.TermRendererOption {
	width, err := strconv.Atoi(os.Getenv("GH_MDWIDTH"))
	if err != nil {
		width = 120
	}
	if w > width {
		w = width
	}
	return ghMarkdown.WithWrap(w)
}

func WithTheme(theme string) glamour.TermRendererOption {
	return ghMarkdown.WithTheme(theme)
}

func WithBaseURL(u string) glamour.TermRendererOption {
	return ghMarkdown.WithBaseURL(u)
}

func Render(text string, opts ...glamour.TermRendererOption) (string, error) {
	return ghMarkdown.Render(text, opts...)
}
