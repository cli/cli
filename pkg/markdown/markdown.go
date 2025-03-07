package markdown

import (
	"os"
	"strconv"

	"github.com/charmbracelet/glamour"
	ghMarkdown "github.com/cli/go-gh/v2/pkg/markdown"
)

func WithoutIndentation() glamour.TermRendererOption {
	return ghMarkdown.WithoutIndentation()
}

// WithWrap is a rendering option that set the character limit for soft
// wrapping the markdown rendering. There is a max limit of 120 characters,
// unless the user overrides with an environment variable.
// If 0 is passed then wrapping is disabled.
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
