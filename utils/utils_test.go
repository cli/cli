package utils

import (
	"testing"
	"time"
)

func TestFuzzyAgo(t *testing.T) {

	cases := map[string]string{
		"1s":       "less than a minute ago",
		"30s":      "less than a minute ago",
		"1m08s":    "about 1 minute ago",
		"15m0s":    "about 15 minutes ago",
		"59m10s":   "about 59 minutes ago",
		"1h10m02s": "about 1 hour ago",
		"15h0m01s": "about 15 hours ago",
		"30h10m":   "about 1 day ago",
		"50h":      "about 2 days ago",
	}

	for duration, expected := range cases {
		d, e := time.ParseDuration(duration)
		if e != nil {
			t.Errorf("failed to create a duration: %s", e)
		}

		fuzzy := FuzzyAgo(d)
		if fuzzy != expected {
			t.Errorf("unexpected fuzzy duration value: %s for %s", fuzzy, duration)
		}
	}
}
