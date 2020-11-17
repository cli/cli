package markdown

import (
	"os"
	"strings"

	"github.com/cli/cli/internal/glamour"
)

func RenderWithOptions(text, style, baseURL string, wrap int) (string, error) {
	// Glamour rendering preserves carriage return characters in code blocks, but
	// we need to ensure that no such characters are present in the output.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	tr, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithBaseURL(baseURL),
		glamour.WithWordWrap(wrap),
	)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
}

func RenderWithBaseURL(text, style, baseURL string) (string, error) {
	return RenderWithOptions(text, style, baseURL, 80)
}

func RenderWithWrap(text, style string, wrap int) (string, error) {
	return RenderWithOptions(text, style, "", wrap)
}

func Render(text, style string) (string, error) {
	return RenderWithOptions(text, style, "", 80)
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
