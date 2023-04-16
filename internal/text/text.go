package text

import (
	"fmt"
	"net/url"
	"regexp"
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

// DisplayURL returns a copy of the string urlStr removing everything except the hostname and path.
// If there is an error parsing urlStr then urlStr is returned without modification.
func DisplayURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	return u.Hostname() + u.Path
}
