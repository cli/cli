package tableprinter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateNonURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "http url",
			input: "http://github.com",
			want:  "http://github.com",
		},
		{
			name:  "https url",
			input: "https://github.com",
			want:  "https://github.com",
		},
		{
			name:  "https url",
			input: "https://github.com",
			want:  "https://github.com",
		},
		{
			name:  "git url",
			input: "git@github.com",
			want:  "git@git...",
		},
		{
			name:  "non-url",
			input: "just some plain text",
			want:  "just so...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateNonURL(10, tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
