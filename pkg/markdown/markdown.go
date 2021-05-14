package markdown

import (
	"fmt"
	"os"
	"regexp"
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

	return render(formatImgTags(text), opts)
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

func formatImgTags(content string) string {
	lines := strings.Split(content, "\n")
	re := regexp.MustCompile(`<img[^>]+\bsrc=["']([^"']+)["']`)
	for i, line := range lines {
		images := re.FindAllStringSubmatch(line, -1)
		for _, element := range images {
			lines[i] = fmt.Sprintf("![Image](%s)", element[1])
		}
	}

	return strings.Join(lines, "\n")
}

var fromEnv = func() string {
	return os.Getenv("GLAMOUR_STYLE")
}
