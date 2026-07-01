package readdir

import (
	"testing"
)

func TestIsTextFile(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", []byte{}, true},
		{"simple text", []byte("hello world"), true},
		{"binary with nul", []byte("hello\x00world"), false},
		{"json", []byte(`{"hello": "world"}`), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTextFile(tt.data); got != tt.want {
				t.Errorf("isTextFile() = %v, want %v", got, tt.want)
			}
		})
	}
}
