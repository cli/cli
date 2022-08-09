package markdown

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

func WithoutIndentation() glamour.TermRendererOption {
	overrides := []byte(`
	  {
			"document": {
				"margin": 0
			},
			"code_block": {
				"margin": 0
			}
	  }`)

	return glamour.WithStylesFromJSONBytes(overrides)
}

func WithWrap(w int) glamour.TermRendererOption {
	return glamour.WithWordWrap(w)
}

type IOStreams interface {
	TerminalTheme() string
}

func WithIO(io IOStreams) glamour.TermRendererOption {
	style := os.Getenv("GLAMOUR_STYLE")
	if style == "" || style == "auto" {
		theme := io.TerminalTheme()
		switch theme {
		case "light", "dark":
			style = theme
		default:
			style = "notty"
		}
	}
	return glamour.WithStylePath(style)
}

func WithBaseURL(u string) glamour.TermRendererOption {
	return glamour.WithBaseURL(u)
}

func Render(text string, opts ...glamour.TermRendererOption) (string, error) {
	// Glamour rendering preserves carriage return characters in code blocks, but
	// we need to ensure that no such characters are present in the output.
	text = strings.ReplaceAll(text, "\r\n", "\n")

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

	opts = append(opts, glamour.WithEmoji(), glamour.WithPreservedNewLines())
	tr, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
}
