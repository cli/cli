package text

import "testing"

func TestReplaceExcessiveWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no replacements",
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
			if got := ReplaceExcessiveWhitespace(tt.input); got != tt.want {
				t.Errorf("ReplaceExcessiveWhitespace() = %v, want %v", got, tt.want)
			}
		})
	}
}
