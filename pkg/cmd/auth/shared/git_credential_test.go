package shared

import (
	"strings"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/prompter"
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

	if _, err := f.Setup("example.com", "monalisa", "PASSWD", false); err != nil {
		t.Errorf("Setup() error = %v", err)
	}
}

func TestSetup_refreshableExternalHelper_declineGh(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	// Only reject is registered: approve must not be called for a refreshable credential, otherwise the stub panics.
	cs.Register(`git credential reject`, 0, "")

	f := GitCredentialFlow{
		Prompter: &prompter.PrompterMock{
			ConfirmFunc: func(_ string, _ bool) (bool, error) {
				return false, nil
			},
		},
		helper: gitcredentials.Helper{Cmd: "osxkeychain"},
		Updater: &gitcredentials.Updater{
			GitClient: &git.Client{GitPath: "some/path/git"},
		},
	}

	warning, err := f.Setup("example.com", "monalisa", "PASSWD", true)
	if err != nil {
		t.Errorf("Setup() error = %v", err)
	}
	if !strings.Contains(warning, "cannot refresh short-lived tokens") {
		t.Errorf("Setup() warning = %q, want it to mention that the helper cannot refresh short-lived tokens", warning)
	}
	if !strings.Contains(warning, "was not stored") {
		t.Errorf("Setup() warning = %q, want it to state that the credential was not stored", warning)
	}
	if !strings.Contains(warning, "gh auth setup-git --hostname example.com") {
		t.Errorf("Setup() warning = %q, want it to suggest running gh auth setup-git for the host", warning)
	}
}

func TestSetup_refreshableExternalHelper_acceptGh(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	// Accepting the prompt configures gh as the helper, so no reject is issued; only the config writes run.
	cs.Register(`git config --global --replace-all credential\.`, 0, "")
	cs.Register(`git config --global --add credential\.`, 0, "", func(args []string) {
		if val := args[len(args)-1]; val != "!/path/to/gh auth git-credential" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})

	f := GitCredentialFlow{
		Prompter: &prompter.PrompterMock{
			ConfirmFunc: func(_ string, _ bool) (bool, error) {
				return true, nil
			},
		},
		helper: gitcredentials.Helper{Cmd: "osxkeychain"},
		HelperConfig: &gitcredentials.HelperConfig{
			SelfExecutablePath: "/path/to/gh",
			GitClient:          &git.Client{GitPath: "some/path/git"},
		},
	}

	warning, err := f.Setup("example.com", "monalisa", "PASSWD", true)
	if err != nil {
		t.Errorf("Setup() error = %v", err)
	}
	if warning != "" {
		t.Errorf("Setup() returned a warning after configuring gh as the helper: %q", warning)
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

	if _, err := f.Setup("github.com", "monalisa", "PASSWD", false); err != nil {
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

	if _, err := f.Setup("example.com", "monalisa", "PASSWD", false); err != nil {
		t.Errorf("Setup() error = %v", err)
	}
}
