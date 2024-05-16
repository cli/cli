package gitcredentials_test

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
)

func TestHelperIsOurs(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{
			name: "blank",
			cmd:  "",
			want: false,
		},
		{
			name: "invalid",
			cmd:  "!",
			want: false,
		},
		{
			name: "osxkeychain",
			cmd:  "osxkeychain",
			want: false,
		},
		{
			name: "looks like gh but isn't",
			cmd:  "gh auth",
			want: false,
		},
		{
			name: "ours",
			cmd:  "!/path/to/gh auth",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := gitcredentials.Helper{Cmd: tt.cmd}
			if got := h.IsOurs(); got != tt.want {
				t.Errorf("IsOurs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHelperIsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{
			name: "blank is not configured",
			cmd:  "",
			want: false,
		},
		{
			name: "anything else is configured",
			cmd:  "unimportant",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := gitcredentials.Helper{Cmd: tt.cmd}
			if got := h.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}
