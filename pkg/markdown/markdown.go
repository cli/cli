package markdown

import (
	"github.com/charmbracelet/glamour"
	ghMarkdown "github.com/cli/go-gh/pkg/markdown"
)

func WithoutIndentation() glamour.TermRendererOption {
	return ghMarkdown.WithoutIndentation()
}

func WithWrap(w int) glamour.TermRendererOption {
	return ghMarkdown.WithWrap(w)
}

type IOStreams interface {
	TerminalTheme() string
}

func WithIO(io IOStreams) glamour.TermRendererOption {
	theme := io.TerminalTheme()
	return ghMarkdown.WithTheme(theme)
}

func WithBaseURL(u string) glamour.TermRendererOption {
	return ghMarkdown.WithBaseURL(u)
}

func Render(text string, opts ...glamour.TermRendererOption) (string, error) {
	return ghMarkdown.Render(text, opts...)
}
