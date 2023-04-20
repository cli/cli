package markdown

import (
	"github.com/charmbracelet/glamour"
	ghMarkdown "github.com/cli/go-gh/v2/pkg/markdown"
)

func WithoutIndentation() glamour.TermRendererOption {
	return ghMarkdown.WithoutIndentation()
}

// WithoutWrap is a rendering option that set the character limit for soft
// wraping the markdown rendering. There is a max limit of 120 characters.
// If 0 is passed then wrapping is disabled.
func WithWrap(w int) glamour.TermRendererOption {
	if w > 120 {
		w = 120
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
