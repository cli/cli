package text

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveExcessiveWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "nothing to remove",
			input: "one two three",
			want:  "one two three",
		},
		{
			name:  "whitespace b-gone",
			input: "\n  one\n\t  two  three\r\n  ",
			want:  "one two three",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveExcessiveWhitespace(tt.input)
			assert.Equal(t, tt.want, got)
		})
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
		d, err := time.Parse(form, createdAt)
		assert.NoError(t, err)
		fuzzy := FuzzyAgoAbbr(now, d)
		assert.Equal(t, expected, fuzzy)
	}
}

func TestFuzzyIn(t *testing.T) {
	const form = "2006-Jan-02 15:04:05"
	now, _ := time.Parse(form, "2020-Nov-22 14:00:00")
	cases := map[string]string{
		"2020-Nov-22 14:00:00": "in less than a minute",
		"2020-Nov-22 14:00:30": "in less than a minute",
		"2020-Nov-22 14:01:00": "in about 1 minute",
		"2020-Nov-22 14:30:00": "in about 30 minutes",
		"2020-Nov-22 15:00:00": "in about 1 hour",
		"2020-Nov-23 02:00:00": "in about 12 hours",
		"2020-Nov-23 14:00:00": "in about 1 day",
		"2020-Dec-07 14:00:00": "in about 15 days",
		"2020-Dec-22 14:00:00": "in about 1 month",
		"2021-Nov-22 14:00:00": "in about 1 year",
		"2025-Nov-22 14:00:00": "in about 5 years",
		// Durations just under a unit boundary round to the nominal value instead of reading one unit short.
		"2020-Nov-22 15:59:59": "in about 2 hours",
		"2021-May-21 13:59:59": "in about 6 months",
		// Durations that round up to the next unit's threshold are reported in the larger unit.
		"2020-Nov-22 14:59:50": "in about 1 hour",
		"2020-Nov-23 13:50:00": "in about 1 day",
		"2020-Dec-22 04:00:00": "in about 1 month",
		"2021-Nov-07 14:00:00": "in about 1 year",
	}
	for target, expected := range cases {
		d, err := time.Parse(form, target)
		require.NoError(t, err)
		assert.Equal(t, expected, FuzzyIn(now, d))
	}
}

func TestFormatSlice(t *testing.T) {
	tests := []struct {
		name        string
		values      []string
		indent      uint
		lineLength  uint
		prependWith string
		appendWith  string
		sort        bool
		wants       string
	}{
		{
			name:       "empty",
			lineLength: 10,
			values:     []string{},
			wants:      "",
		},
		{
			name:       "empty with indent",
			lineLength: 10,
			indent:     2,
			values:     []string{},
			wants:      "  ",
		},
		{
			name:       "single",
			lineLength: 10,
			values:     []string{"foo"},
			wants:      "foo",
		},
		{
			name:       "single with indent",
			lineLength: 10,
			indent:     2,
			values:     []string{"foo"},
			wants:      "  foo",
		},
		{
			name:       "long single with indent",
			lineLength: 10,
			indent:     2,
			values:     []string{"some-long-value"},
			wants:      "  some-long-value",
		},
		{
			name:       "exact line length",
			lineLength: 4,
			values:     []string{"a", "b"},
			wants:      "a, b",
		},
		{
			name:       "values longer than line length",
			lineLength: 4,
			values:     []string{"long-value", "long-value"},
			wants:      "long-value,\nlong-value",
		},
		{
			name:       "zero line length (no wrapping expected)",
			lineLength: 0,
			values:     []string{"foo", "bar"},
			wants:      "foo, bar",
		},
		{
			name:       "simple",
			lineLength: 10,
			values:     []string{"foo", "bar", "baz", "foo", "bar", "baz"},
			wants:      "foo, bar,\nbaz, foo,\nbar, baz",
		},
		{
			name:        "simple, surrounded",
			lineLength:  13,
			prependWith: "<",
			appendWith:  ">",
			values:      []string{"foo", "bar", "baz", "foo", "bar", "baz"},
			wants:       "<foo>, <bar>,\n<baz>, <foo>,\n<bar>, <baz>",
		},
		{
			name:       "sort",
			lineLength: 99,
			sort:       true,
			values:     []string{"c", "b", "a"},
			wants:      "a, b, c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wants, FormatSlice(tt.values, tt.lineLength, tt.indent, tt.prependWith, tt.appendWith, tt.sort))
		})
	}
}

func TestDisplayURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "simple",
			url:  "https://github.com/cli/cli/issues/9470",
			want: "https://github.com/cli/cli/issues/9470",
		},
		{
			name: "without scheme",
			url:  "github.com/cli/cli/issues/9470",
			want: "https://github.com/cli/cli/issues/9470",
		},
		{
			name: "with query param and anchor",
			url:  "https://github.com/cli/cli/issues/9470?q=is:issue#issue-command",
			want: "https://github.com/cli/cli/issues/9470",
		},
		{
			name: "preserve http protocol use despite insecure",
			url:  "http://github.com/cli/cli/issues/9470",
			want: "http://github.com/cli/cli/issues/9470",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DisplayURL(tt.url))
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{2048, "2.0 KB"},
		{10240, "10.0 KB"},
		{524288, "512.0 KB"},
		{1048576, "1.0 MB"},
		{1572864, "1.5 MB"},
		{5242880, "5.0 MB"},
		{1073741824, "1.0 GB"},
		{1610612736, "1.5 GB"},
		{1099511627776, "1.0 TB"},
		{1125899906842624, "1.0 PB"},
		{1152921504606846976, "1024.0 PB"}, // 1 EB clamps to the largest known unit
		{math.MaxInt64, "8192.0 PB"},       // maximum int never indexes past PB
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatSize(tt.n))
	}
}
