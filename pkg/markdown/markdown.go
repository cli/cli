package markdown

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
)

type RenderOpts []glamour.TermRendererOption

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

func WithoutWrap() glamour.TermRendererOption {
	return glamour.WithWordWrap(0)
}

func render(text string, opts RenderOpts) (string, error) {
	// Glamour rendering preserves carriage return characters in code blocks, but
	// we need to ensure that no such characters are present in the output.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	tr, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
}

func Render(text, style string) (string, error) {
	opts := RenderOpts{
		glamour.WithStylePath(style),
		glamour.WithEmoji(),
	}

	return render(text, opts)
}

func RenderWithOpts(text, style string, opts RenderOpts) (string, error) {
	defaultOpts := RenderOpts{
		glamour.WithStylePath(style),
		glamour.WithEmoji(),
	}
	opts = append(defaultOpts, opts...)

	return render(text, opts)
}

func RenderWithBaseURL(text, style, baseURL string) (string, error) {
	opts := RenderOpts{
		glamour.WithStylePath(style),
		glamour.WithEmoji(),
		glamour.WithBaseURL(baseURL),
	}

	return render(text, opts)
}

func RenderWithWrap(text, style string, wrap int) (string, error) {
	opts := RenderOpts{
		glamour.WithStylePath(style),
		glamour.WithEmoji(),
		glamour.WithWordWrap(wrap),
	}

	return render(text, opts)
}

func GetStyle(defaultStyle string) string {
	style := fromEnv()
	if style != "" && style != "auto" {
		return style
	}

	if defaultStyle == "light" || defaultStyle == "dark" {
		return defaultStyle
	}

	return "notty"
}

var fromEnv = func() string {
	return os.Getenv("GLAMOUR_STYLE")
}
