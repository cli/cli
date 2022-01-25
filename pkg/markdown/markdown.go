package markdown

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
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

	opts = append(opts, glamour.WithEmoji(), glamour.WithPreservedNewLines())
	tr, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
}
