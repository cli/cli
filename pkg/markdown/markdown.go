package markdown

import (
	"os"

	"github.com/charmbracelet/glamour"
	ghMarkdown "github.com/cli/go-gh/pkg/markdown"
	"golang.org/x/term"
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
	// Set default word wrap limit to 120. Glamour default is 80, which causes
	// problems with hard-wrapped text and blockquoted text in issues and PRs.
	width := 120

	// If terminal width less than default width, set width to terminal width
	if term.IsTerminal(int(os.Stdout.Fd())) {
		w, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil && int(w) > 0 && int(w) < width {
			width = int(w)
		}
	}

	// Prepend word wrap width to opts, which allows it to be overridden
	opts = append([]glamour.TermRendererOption{glamour.WithWordWrap(width)}, opts...)

	return ghMarkdown.Render(text, opts...)
}
