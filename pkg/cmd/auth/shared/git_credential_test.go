package shared

import (
	"testing"

	"github.com/cli/cli/internal/run"
)

func TestGitCredentialSetup_configureExisting(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git credential reject`, 0, "")
	cs.Register(`git credential approve`, 0, "")

	if err := GitCredentialSetup("example.com", "monalisa", "PASSWD", "osxkeychain"); err != nil {
		t.Errorf("GitCredentialSetup() error = %v", err)
	}
}

func TestGitCredentialSetup_setOurs(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config --global credential\.https://example\.com\.helper`, 0, "", func(args []string) {
		if val := args[len(args)-1]; val != "!gh auth git-credential" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})

	if err := GitCredentialSetup("example.com", "monalisa", "PASSWD", ""); err != nil {
		t.Errorf("GitCredentialSetup() error = %v", err)
	}
}

func Test_isOurCredentialHelper(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{
			name: "blank",
			arg:  "",
			want: false,
		},
		{
			name: "invalid",
			arg:  "!",
			want: false,
		},
		{
			name: "osxkeychain",
			arg:  "osxkeychain",
			want: false,
		},
		{
			name: "looks like gh but isn't",
			arg:  "gh auth",
			want: false,
		},
		{
			name: "ours",
			arg:  "!/path/to/gh auth",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOurCredentialHelper(tt.arg); got != tt.want {
				t.Errorf("isOurCredentialHelper() = %v, want %v", got, tt.want)
			}
		})
	}
}
