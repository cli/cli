package text

import (
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Copied from: https://github.com/asaskevich/govalidator
func CamelToKebab(str string) string {
	var output []rune
	var segment []rune
	for _, r := range str {
		if !unicode.IsLower(r) && string(r) != "-" && !unicode.IsNumber(r) {
			output = addSegment(output, segment)
			segment = nil
		}
		segment = append(segment, unicode.ToLower(r))
	}
	output = addSegment(output, segment)
	return string(output)
}

func addSegment(inrune, segment []rune) []rune {
	if len(segment) == 0 {
		return inrune
	}
	if len(inrune) != 0 {
		inrune = append(inrune, '-')
	}
	inrune = append(inrune, segment...)
	return inrune
}

func Title(str string) string {
	c := cases.Title(language.English)
	return c.String(str)
}
