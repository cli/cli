package text

import (
	"regexp"
	"strings"
)

var lineRE = regexp.MustCompile(`(?m)^`)

func Indent(s, indent string) string {
	if len(strings.TrimSpace(s)) == 0 {
		return s
	}
	return lineRE.ReplaceAllLiteralString(s, indent)
}
