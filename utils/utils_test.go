package utils

import (
	"testing"
	"time"
)

func TestFuzzyAgo(t *testing.T) {
	cases := map[string]string{
		"1s":         "less than a minute ago",
		"30s":        "less than a minute ago",
		"1m08s":      "about 1 minute ago",
		"15m0s":      "about 15 minutes ago",
		"59m10s":     "about 59 minutes ago",
		"1h10m02s":   "about 1 hour ago",
		"15h0m01s":   "about 15 hours ago",
		"30h10m":     "about 1 day ago",
		"50h":        "about 2 days ago",
		"720h05m":    "about 1 month ago",
		"3000h10m":   "about 4 months ago",
		"8760h59m":   "about 1 year ago",
		"17601h59m":  "about 2 years ago",
		"262800h19m": "about 30 years ago",
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

func TestFuzzyAgoAbbr(t *testing.T) {
	const form = "2006-Jan-02 15:04:05"
	now, _ := time.Parse(form, "2020-Nov-22 14:00:00")

	cases := map[string]string{
		"2020-Nov-22 14:00:00": "0m",
		"2020-Nov-22 13:59:00": "1m",
		"2020-Nov-22 13:30:00": "30m",
		"2020-Nov-22 13:00:00": "1h",
		"2020-Nov-22 02:00:00": "12h",
		"2020-Nov-21 14:00:00": "1d",
		"2020-Nov-07 14:00:00": "15d",
		"2020-Oct-24 14:00:00": "29d",
		"2020-Oct-23 14:00:00": "Oct 23, 2020",
		"2019-Nov-22 14:00:00": "Nov 22, 2019",
	}

	for createdAt, expected := range cases {
		d, _ := time.Parse(form, createdAt)
		fuzzy := FuzzyAgoAbbr(now, d)
		if fuzzy != expected {
			t.Errorf("unexpected fuzzy duration abbr value: %s for %s", fuzzy, createdAt)
		}
	}
}

func TestLevenshteinDistance(t *testing.T) {
	cases := []struct {
		s        string
		t        string
		expected int
	}{
		{"", "", 0},
		{"", "s", 1},
		{"k", "", 1},
		{"k", "s", 1},
		{"kitten", "kitten", 0},
		{"kitten", "sitting", 3},
		{"cli/cli", "cli/cii", 1},
	}
	for _, c := range cases {
		got := LevenshteinDistance(c.s, c.t)
		if got != c.expected {
			t.Errorf("unexpected LevenshteinDistance: expected %d, got %d, for %s, %s",
				c.expected,
				got,
				c.s,
				c.t,
			)
		}
	}
}

func TestFilterBySimilarity(t *testing.T) {
	cases := []struct {
		name     string
		s        string
		cands    []string
		opt      FilterBySimilarityOpts
		expected []string
	}{
		{
			"test normal",
			"cli/clii",
			[]string{
				"cli/cli",
				"cli/go-gh",
				"cli/scoop-gh",
				"cli/browser",
				"cli/gh-extension-precompile",
				"cli/oauth",
				"cli/safeexec",
				"cli/crypto",
				"cli/shurcooL-graphql",
			},
			DefaultFilterBySimilarityOpts,
			[]string{
				"cli/cli",
			},
		},
		{
			"test IgnoreCase: true(default)",
			"cli/GO-GH",
			[]string{
				"cli/cli",
				"cli/go-gh",
				"cli/scoop-gh",
				"cli/browser",
				"cli/gh-extension-precompile",
				"cli/oauth",
				"cli/safeexec",
				"cli/crypto",
				"cli/shurcooL-graphql",
			},
			DefaultFilterBySimilarityOpts,
			[]string{
				"cli/go-gh",
			},
		},
		{
			"test IgnoreCase: false",
			"cli/GO-GH",
			[]string{
				"cli/cli",
				"cli/go-gh",
				"cli/scoop-gh",
				"cli/browser",
				"cli/gh-extension-precompile",
				"cli/oauth",
				"cli/safeexec",
				"cli/crypto",
				"cli/shurcooL-graphql",
			},
			FilterBySimilarityOpts{
				IgnoreCase: false,
			},
			[]string{},
		},
		{
			"test Limit, DistanceLimit",
			"cli/GO-GH",
			[]string{
				"cli/cli",
				"cli/go-gh",
				"cli/scoop-gh",
				"cli/browser",
				"cli/gh-extension-precompile",
				"cli/oauth",
				"cli/safeexec",
				"cli/crypto",
				"cli/shurcooL-graphql",
			},
			FilterBySimilarityOpts{
				IgnoreCase:    true,
				DistanceLimit: 4,
				Limit:         2,
			},
			[]string{
				"cli/go-gh",
				"cli/scoop-gh",
			},
		},
	}
	for _, c := range cases {
		got := FilterBySimilarity(c.s, c.cands, c.opt)
		if len(got) != len(c.expected) {
			t.Errorf(`unexpected result of "%s": length not equal`, c.name)
		}
		for i := 0; i < len(got); i++ {
			g := got[i]
			e := c.expected[i]
			if g != e {
				t.Errorf(`unexpected of "%s": expected %s, got %s`,
					c.name,
					e,
					g,
				)
			}

		}
	}
}
