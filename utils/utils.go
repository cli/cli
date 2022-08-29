package utils

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
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

// Maximum length of a URL: 8192 bytes
func ValidURL(urlStr string) bool {
	return len(urlStr) < 8192
}

func StringInSlice(a string, slice []string) bool {
	for _, b := range slice {
		if b == a {
			return true
		}
	}
	return false
}

func IsDebugEnabled() (bool, string) {
	debugValue, isDebugSet := os.LookupEnv("GH_DEBUG")
	legacyDebugValue := os.Getenv("DEBUG")

	if !isDebugSet {
		switch legacyDebugValue {
		case "true", "1", "yes", "api":
			return true, legacyDebugValue
		default:
			return false, legacyDebugValue
		}
	}

	switch debugValue {
	case "false", "0", "no", "":
		return false, debugValue
	default:
		return true, debugValue
	}
}

func LevenshteinDistance(s, t string) int {
	if len(s) == 0 {
		return len(t)
	}
	if len(t) == 0 {
		return len(s)
	}
	if s[0] == t[0] {
		return LevenshteinDistance(s[1:], t[1:])
	}

	vals := []int{
		LevenshteinDistance(s[1:], t[1:]),
		LevenshteinDistance(s[1:], t),
		LevenshteinDistance(s, t[1:]),
	}

	sort.Ints(vals)

	min := vals[0]

	return 1 + min
}

func LevenshteinDistanceIter(s, t string) int {
	m := make([][]int, len(s)+1)
	for i := 0; i <= len(s); i++ {
		m[i] = make([]int, len(t)+1)
	}

	for i := 1; i <= len(s); i++ {
		m[i][0] = i
	}

	for j := 1; j <= len(t); j++ {
		m[0][j] = j
	}

	for j := 1; j <= len(t); j++ {
		for i := 1; i <= len(s); i++ {

			cost := 0
			if s[i-1] != t[j-1] {
				cost = 1
			}

			vals := []int{
				m[i-1][j] + 1,      // deletion
				m[i][j-1] + 1,      // insertion
				m[i-1][j-1] + cost, // substitution
			}

			sort.Ints(vals)
			m[i][j] = vals[0]
		}
	}
	return m[len(s)][len(t)]
}

func LevenshteinDistanceIter2(s, t string) int {
	v0 := make([]int, len(t)+1)
	v1 := make([]int, len(t)+1)
	for i := 0; i <= len(t); i++ {
		v0[i] = i
	}
	for i := 0; i < len(s); i++ {
		v1[0] = i + 1
		for j := 0; j < len(t); j++ {
			cost := 0
			if s[i] != t[j] {
				cost = 1
			}
			vals := []int{
				v0[j+1] + 1,  // deletion
				v1[j] + 1,    // insertion
				v0[j] + cost, // substitution

			}
			sort.Ints(vals)
			v1[j+1] = vals[0]
		}
		copy(v0, v1)
	}
	return v0[len(t)]
}
