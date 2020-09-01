package text

import (
	"regexp"
	"strings"
)

var ws = regexp.MustCompile(`\s+`)

func ReplaceExcessiveWhitespace(s string) string {
	return ws.ReplaceAllString(strings.TrimSpace(s), " ")
}
