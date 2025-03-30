package shared

import (
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
)

func TestSetup_configureExisting(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git credential reject`, 0, "")
	cs.Register(`git credential approve`, 0, "")

	f := GitCredentialFlow{
		helper: gitcredentials.Helper{Cmd: "osxkeychain"},
		Updater: &gitcredentials.Updater{
			GitClient: &git.Client{GitPath: "some/path/git"},
		},
	}

	if err := f.Setup("example.com", "monalisa", "PASSWD"); err != nil {
		t.Errorf("Setup() error = %v", err)
	}
}

func TestGitCredentialsSetup_setOurs_GH(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config --global --replace-all credential\.`, 0, "", func(args []string) {
		if key := args[len(args)-2]; key != "credential.https://github.com.helper" {
			t.Errorf("git config key was %q", key)
		}
		if val := args[len(args)-1]; val != "" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})
	cs.Register(`git config --global --add credential\.`, 0, "", func(args []string) {
		if key := args[len(args)-2]; key != "credential.https://github.com.helper" {
			t.Errorf("git config key was %q", key)
		}
		if val := args[len(args)-1]; val != "!/path/to/gh auth git-credential" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})
	cs.Register(`git config --global --replace-all credential\.`, 0, "", func(args []string) {
		if key := args[len(args)-2]; key != "credential.https://gist.github.com.helper" {
			t.Errorf("git config key was %q", key)
		}
		if val := args[len(args)-1]; val != "" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})
	cs.Register(`git config --global --add credential\.`, 0, "", func(args []string) {
		if key := args[len(args)-2]; key != "credential.https://gist.github.com.helper" {
			t.Errorf("git config key was %q", key)
		}
		if val := args[len(args)-1]; val != "!/path/to/gh auth git-credential" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})

	f := GitCredentialFlow{
		helper: gitcredentials.Helper{},
		HelperConfig: &gitcredentials.HelperConfig{
			SelfExecutablePath: "/path/to/gh",
			GitClient:          &git.Client{GitPath: "some/path/git"},
		},
	}

	if err := f.Setup("github.com", "monalisa", "PASSWD"); err != nil {
		t.Errorf("Setup() error = %v", err)
	}

}

func TestSetup_setOurs_nonGH(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config --global --replace-all credential\.`, 0, "", func(args []string) {
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
		helper: gitcredentials.Helper{},
		HelperConfig: &gitcredentials.HelperConfig{
			SelfExecutablePath: "/path/to/gh",
			GitClient:          &git.Client{GitPath: "some/path/git"},
		},
	}

	if err := f.Setup("example.com", "monalisa", "PASSWD"); err != nil {
		t.Errorf("Setup() error = %v", err)
	}
}
