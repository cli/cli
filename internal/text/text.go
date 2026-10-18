package text

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var whitespaceRE = regexp.MustCompile(`\s+`)
var indentRE = regexp.MustCompile(`(?m)^`)

func Indent(s, indent string) string {
	if len(strings.TrimSpace(s)) == 0 {
		return s
	}
	return indentRE.ReplaceAllLiteralString(s, indent)
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
	var width int
	var inEscapeSequence bool
	for _, r := range s {
		if r == '\x1b' {
			inEscapeSequence = true
		} else if inEscapeSequence {
			if isEscapeTerminator(r) {
				inEscapeSequence = false
			}
		} else {
			width += runewidth.RuneWidth(r)
		}
	}
	return width
}

func Truncate(maxWidth int, s string) string {
	if DisplayWidth(s) <= maxWidth {
		return s
	}
	tail := ""
	if maxWidth >= 5 {
		tail = "..."
	}
	tailWidth := DisplayWidth(tail)
	if maxWidth < tailWidth {
		return tail
	}

	widthLimit := maxWidth - tailWidth
	var result strings.Builder
	var escapeSequence strings.Builder
	var lastColorSequence strings.Builder
	var currentWidth int
	var inEscapeSequence bool
	var sequenceChanged bool
	for _, r := range s {
		if r == '\x1b' {
			inEscapeSequence = true
			sequenceChanged = true
			escapeSequence.WriteRune(r)
		} else if inEscapeSequence {
			escapeSequence.WriteRune(r)
			if isEscapeTerminator(r) {
				inEscapeSequence = false
				sequence := escapeSequence.String()
				result.WriteString(sequence)
				escapeSequence.Reset()
				if strings.HasSuffix(sequence, "[0m") {
					lastColorSequence.Reset()
					sequenceChanged = false
				} else if r == 'm' {
					lastColorSequence.WriteString(sequence)
				}
			}
		} else {
			currentWidth += runewidth.RuneWidth(r)
			if currentWidth > widthLimit {
				result.WriteString(tail)
				if lastColorSequence.Len() > 0 && sequenceChanged {
					result.WriteString("\x1b[0m")
				}
				return result.String()
			}
			result.WriteRune(r)
		}
	}
	result.WriteString(escapeSequence.String())
	if DisplayWidth(result.String()) < maxWidth {
		result.WriteByte(' ')
	}
	return result.String()
}

func isEscapeTerminator(r rune) bool {
	return r >= 0x40 && r <= 0x5a || r >= 0x61 && r <= 0x7a
}

func Pluralize(num int, thing string) string {
	if num == 1 {
		return fmt.Sprintf("%d %s", num, thing)
	}
	return fmt.Sprintf("%d %ss", num, thing)
}

func FuzzyAgo(a, b time.Time) string {
	ago := a.Sub(b)
	if ago < time.Minute {
		return "less than a minute ago"
	}
	if ago < time.Hour {
		return fmtDuration(int(ago.Minutes()), "minute")
	}
	if ago < 24*time.Hour {
		return fmtDuration(int(ago.Hours()), "hour")
	}
	if ago < 30*24*time.Hour {
		return fmtDuration(int(ago.Hours())/24, "day")
	}
	if ago < 365*24*time.Hour {
		return fmtDuration(int(ago.Hours())/24/30, "month")
	}
	return fmtDuration(int(ago.Hours()/24/365), "year")
}

func fmtDuration(amount int, unit string) string {
	return fmt.Sprintf("about %s ago", Pluralize(amount, unit))
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
	removeMarks := runes.Remove(runes.In(unicode.Mn))
	normalize := transform.Chain(norm.NFD, removeMarks, norm.NFC)
	result, _, err := transform.String(normalize, value)
	if err != nil {
		return value
	}
	return result
}

func PadRight(maxWidth int, s string) string {
	if padWidth := maxWidth - DisplayWidth(s); padWidth > 0 {
		s += strings.Repeat(" ", padWidth)
	}
	return s
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

// FormatSize formats a byte count using binary units (B, KB, MB, GB, TB, PB).
// Values below a kilobyte are shown as whole bytes; larger values are shown with
// one decimal place of precision.
func FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	units := []string{"KB", "MB", "GB", "TB", "PB"}

	// Stop at the largest known unit so an out-of-range index can never occur,
	// even for byte counts beyond a petabyte.
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < len(units)-1; v /= unit {
		div *= unit
		exp++
	}

	value := float64(n) / float64(div)
	return fmt.Sprintf("%.1f %s", value, units[exp])
}
