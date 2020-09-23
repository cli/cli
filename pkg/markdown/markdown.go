package markdown

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
)

func Render(text string, bgColor string) (string, error) {
	// Glamour rendering preserves carriage return characters in code blocks, but
	// we need to ensure that no such characters are present in the output.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	tr, err := glamour.NewTermRenderer(
		glamour.WithStylePath(getEnvironmentStyle(bgColor)),
		// glamour.WithBaseURL(""),  // TODO: make configurable
		// glamour.WithWordWrap(80), // TODO: make configurable
	)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
}

func getEnvironmentStyle(bgColor string) string {
	style := os.Getenv("GLAMOUR_STYLE")
	if style != "" && style != "auto" {
		return style
	}

	if bgColor == "light" || bgColor == "dark" {
		return bgColor
	}

	return "notty"
}
