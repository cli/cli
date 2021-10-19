package utils

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
)

func Pluralize(num int, thing string) string {
	if num == 1 {
		return fmt.Sprintf("%d %s", num, thing)
	}
	return fmt.Sprintf("%d %ss", num, thing)
}

func fmtDuration(amount int, unit string) string {
	return fmt.Sprintf("about %s ago", Pluralize(amount, unit))
}

func FuzzyAgo(ago time.Duration) string {
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

func FuzzyAgoAbbr(now time.Time, createdAt time.Time) string {
	ago := now.Sub(createdAt)

	if ago < time.Hour {
		return fmt.Sprintf("%d%s", int(ago.Minutes()), "m")
	}
	if ago < 24*time.Hour {
		return fmt.Sprintf("%d%s", int(ago.Hours()), "h")
	}
	if ago < 30*24*time.Hour {
		return fmt.Sprintf("%d%s", int(ago.Hours())/24, "d")
	}

	return createdAt.Format("Jan _2, 2006")
}

func Humanize(s string) string {
	// Replaces - and _ with spaces.
	replace := "_-"
	h := func(r rune) rune {
		if strings.ContainsRune(replace, r) {
			return ' '
		}
		return r
	}

	return strings.Map(h, s)
}

func IsURL(s string) bool {
	return strings.HasPrefix(s, "http:/") || strings.HasPrefix(s, "https:/")
}

func DisplayURL(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	return u.Hostname() + u.Path
}

// ValidURL ccecks if maximum length of a URL: 8192 bytes
func ValidURL(urlStr string) bool {
	return len(urlStr) < 8192
}

// Camelize transforms string to CamelCase
func Camelize(s string) string {
	var b strings.Builder
	capitalizeNext := false
	for i, r := range s {
		if r == ' ' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			b.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
		} else if i == 0 {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
