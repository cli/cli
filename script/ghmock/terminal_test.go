package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeys(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "text and a named key", in: "my-repo{enter}", want: "my-repo\r"},
		{name: "arrows and space", in: "{down}{down}{space}{enter}", want: "\x1b[B\x1b[B \r"},
		{name: "key names ignore case", in: "{Enter}", want: "\r"},
		{name: "a doubled brace types a brace", in: "a{{b}", want: "a{b}"},
		{name: "nothing", in: "", want: ""},
		{name: "a brace without its end", in: "{enter", wantErr: `--keys "{enter" has a { without a }; type a brace as {{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseKeys(tt.in)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
