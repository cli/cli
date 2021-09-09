package shared

import (
	"testing"

	"github.com/cli/cli/v2/internal/run"
)

func TestGitCredentialSetup_configureExisting(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git credential reject`, 0, "")
	cs.Register(`git credential approve`, 0, "")

	f := GitCredentialFlow{
		Executable: "gh",
		helper:     "osxkeychain",
	}

	if err := f.gitCredentialSetup("example.com", "monalisa", "PASSWD"); err != nil {
		t.Errorf("GitCredentialSetup() error = %v", err)
	}
}

func TestGitCredentialSetup_setOurs(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config --global credential\.`, 0, "", func(args []string) {
		if key := args[len(args)-2]; key != "credential.https://example.com.helper" {
			t.Errorf("git config key was %q", key)
		}
		if val := args[len(args)-1]; val != "" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})
	cs.Register(`git config --global --add credential\.`, 0, "", func(args []string) {
		if key := args[len(args)-2]; key != "credential.https://example.com.helper" {
			t.Errorf("git config key was %q", key)
		}
		if val := args[len(args)-1]; val != "!/path/to/gh auth git-credential" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})

	f := GitCredentialFlow{
		Executable: "/path/to/gh",
		helper:     "",
	}

	if err := f.gitCredentialSetup("example.com", "monalisa", "PASSWD"); err != nil {
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
