package text

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/text"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var whitespaceRE = regexp.MustCompile(`\s+`)

func Indent(s, indent string) string {
	return text.Indent(s, indent)
}

// Title returns a copy of the string s with all Unicode letters that begin words mapped to their Unicode title case.
func Title(s string) string {
	c := cases.Title(language.English)
	return c.String(s)
}

// RemoveExcessiveWhitespace returns a copy of the string s with excessive whitespace removed.
func RemoveExcessiveWhitespace(s string) string {
	return whitespaceRE.ReplaceAllString(strings.TrimSpace(s), " ")
}

func DisplayWidth(s string) int {
	return text.DisplayWidth(s)
}

func Truncate(maxWidth int, s string) string {
	return text.Truncate(maxWidth, s)
}

func Pluralize(num int, thing string) string {
	return text.Pluralize(num, thing)
}

func FuzzyAgo(a, b time.Time) string {
	return text.RelativeTimeAgo(a, b)
}

// FuzzyAgoAbbr is an abbreviated version of FuzzyAgo. It returns a human readable string of the
// time duration between a and b that is estimated to the nearest unit of time.
func FuzzyAgoAbbr(a, b time.Time) string {
	ago := a.Sub(b)

	if ago < time.Hour {
		return fmt.Sprintf("%d%s", int(ago.Minutes()), "m")
	}
	if ago < 24*time.Hour {
		return fmt.Sprintf("%d%s", int(ago.Hours()), "h")
	}
	if ago < 30*24*time.Hour {
		return fmt.Sprintf("%d%s", int(ago.Hours())/24, "d")
	}

	return b.Format("Jan _2, 2006")
}

// DisplayURL returns a copy of the string urlStr removing everything except the scheme, hostname, and path.
// If the scheme is not specified, "https" is assumed.
// If there is an error parsing urlStr then urlStr is returned without modification.
func DisplayURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + u.Hostname() + u.Path
}

// RemoveDiacritics returns the input value without "diacritics", or accent marks
func RemoveDiacritics(value string) string {
	return text.RemoveDiacritics(value)
}

func PadRight(maxWidth int, s string) string {
	return text.PadRight(maxWidth, s)
}

// FormatSlice concatenates elements of the given string slice into a
// well-formatted, possibly multiline, string with specific line length limit.
// Elements can be optionally surrounded by custom strings (e.g., quotes or
// brackets). If the lineLength argument is non-positive, no line length limit
// will be applied.
func FormatSlice(values []string, lineLength uint, indent uint, prependWith string, appendWith string, sort bool) string {
	if lineLength <= 0 {
		lineLength = math.MaxInt
	}

	sortedValues := values
	if sort {
		sortedValues = slices.Clone(values)
		slices.Sort(sortedValues)
	}

	pre := strings.Repeat(" ", int(indent))
	if len(sortedValues) == 0 {
		return pre
	} else if len(sortedValues) == 1 {
		return pre + sortedValues[0]
	}

	builder := strings.Builder{}
	currentLineLength := 0
	sep := ","
	ws := " "

	for i := 0; i < len(sortedValues); i++ {
		v := prependWith + sortedValues[i] + appendWith
		isLast := i == -1+len(sortedValues)

		if currentLineLength == 0 {
			builder.WriteString(pre)
			builder.WriteString(v)
			currentLineLength += len(v)
			if !isLast {
				builder.WriteString(sep)
				currentLineLength += len(sep)
			}
		} else {
			if !isLast && currentLineLength+len(ws)+len(v)+len(sep) > int(lineLength) ||
				isLast && currentLineLength+len(ws)+len(v) > int(lineLength) {
				currentLineLength = 0
				builder.WriteString("\n")
				i--
				continue
			}

			builder.WriteString(ws)
			builder.WriteString(v)
			currentLineLength += len(ws) + len(v)
			if !isLast {
				builder.WriteString(sep)
				currentLineLength += len(sep)
			}
		}
	}
	return builder.String()
}
