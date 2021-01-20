package markdown

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
)

func Render(text, style string, baseURL string) (string, error) {
	// Glamour rendering preserves carriage return characters in code blocks, but
	// we need to ensure that no such characters are present in the output.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	tr, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithBaseURL(baseURL),
		// glamour.WithWordWrap(80), // TODO: make configurable
	)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
}

func RenderWrap(text, style string, wrap int) (string, error) {
	// Glamour rendering preserves carriage return characters in code blocks, but
	// we need to ensure that no such characters are present in the output.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	tr, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		// glamour.WithBaseURL(""),  // TODO: make configurable
		glamour.WithWordWrap(wrap),
	)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
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
