package text

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestRemoveDiacritics(t *testing.T) {
	tests := [][]string{
		// no diacritics
		{"e", "e"},
		{"و", "و"},
		{"И", "И"},
		{"ж", "ж"},
		{"私", "私"},
		{"万", "万"},

		// diacritics test sets
		{"à", "a"},
		{"é", "e"},
		{"è", "e"},
		{"ô", "o"},
		{"ᾳ", "α"},
		{"εͅ", "ε"},
		{"ῃ", "η"},
		{"ιͅ", "ι"},

		{"ؤ", "و"},

		{"ā", "a"},
		{"č", "c"},
		{"ģ", "g"},
		{"ķ", "k"},
		{"ņ", "n"},
		{"š", "s"},
		{"ž", "z"},

		{"ŵ", "w"},
		{"ŷ", "y"},
		{"ä", "a"},
		{"ÿ", "y"},
		{"á", "a"},
		{"ẁ", "w"},
		{"ỳ", "y"},
		{"ō", "o"},

		// full words
		{"Miķelis", "Mikelis"},
		{"François", "Francois"},
		{"žluťoučký", "zlutoucky"},
		{"învățătorița", "invatatorita"},
		{"Kękę przy łóżku", "Keke przy łozku"},
	}

	for _, tt := range tests {
		t.Run(RemoveDiacritics(tt[0]), func(t *testing.T) {
			assert.Equal(t, tt[1], RemoveDiacritics(tt[0]))
		})
	}
}
