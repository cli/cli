package shared

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/cli/cli/v2/pkg/iostreams"
)

// stubHelperConfig is a test double for the HelperConfig interface that returns a fixed configured helper and records
// whether gh was configured as the credential helper.
type stubHelperConfig struct {
	helper        gitcredentials.Helper
	helperErr     error
	configuredOur bool
}

func (s *stubHelperConfig) ConfigureOurs(string) error {
	s.configuredOur = true
	return nil
}

func (s *stubHelperConfig) ConfiguredHelper(string) (gitcredentials.Helper, error) {
	return s.helper, s.helperErr
}

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
	if !strings.Contains(warning, "cannot refresh expiring tokens") {
		t.Errorf("Setup() warning = %q, want it to mention that the helper cannot refresh expiring tokens", warning)
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

func TestPrompt_expiring_configure(t *testing.T) {
	var gotMessage string
	ios, _, _, stderr := iostreams.Test()
	hc := &stubHelperConfig{helper: gitcredentials.Helper{Cmd: "osxkeychain"}}
	f := GitCredentialFlow{
		IO: ios,
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(message, _ string, options []string) (int, error) {
				gotMessage = message
				return slices.Index(options, "Set up gh as your Git credential helper (recommended)"), nil
			},
		},
		HelperConfig: hc,
	}

	err := f.Prompt("github.com", true)
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if !f.ShouldSetup() {
		t.Error("ShouldSetup() = false, want true after choosing to configure gh")
	}
	if !slices.Contains(f.Scopes(), "workflow") {
		t.Errorf("Scopes() = %v, want it to include workflow", f.Scopes())
	}
	if !strings.Contains(gotMessage, "How would you like to proceed?") {
		t.Errorf("prompt message = %q, want the short decision prompt", gotMessage)
	}
	if !strings.Contains(stderr.String(), "expiring token") {
		t.Errorf("warning = %q, want it to explain the expiring token consequence", stderr.String())
	}
}

func TestPrompt_expiring_decline(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := GitCredentialFlow{
		IO: ios,
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(_, _ string, options []string) (int, error) {
				return slices.Index(options, "Continue without it (you manage Git credentials yourself)"), nil
			},
		},
		HelperConfig: &stubHelperConfig{helper: gitcredentials.Helper{Cmd: "osxkeychain"}},
	}

	err := f.Prompt("github.com", true)
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if f.ShouldSetup() {
		t.Error("ShouldSetup() = true, want false after declining to configure gh")
	}
}

func TestPrompt_expiring_stop(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := GitCredentialFlow{
		IO: ios,
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(_, _ string, options []string) (int, error) {
				return slices.Index(options, "Cancel"), nil
			},
		},
		HelperConfig: &stubHelperConfig{helper: gitcredentials.Helper{Cmd: "osxkeychain"}},
	}

	err := f.Prompt("github.com", true)
	if !errors.Is(err, ErrExpiringSetupCredHelperCancelled) {
		t.Errorf("Prompt() error = %v, want ErrExpiringSetupCredHelperCancelled", err)
	}
	if f.ShouldSetup() {
		t.Error("ShouldSetup() = true, want false after stopping the sign-in")
	}
}

func TestPrompt_expiring_alreadyOurs(t *testing.T) {
	selectCalled := false
	f := GitCredentialFlow{
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(_, _ string, options []string) (int, error) {
				selectCalled = true
				return slices.Index(options, "Set up gh as your Git credential helper (recommended)"), nil
			},
		},
		HelperConfig: &stubHelperConfig{helper: gitcredentials.Helper{Cmd: "!/path/to/gh auth git-credential"}},
	}

	err := f.Prompt("github.com", true)
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if selectCalled {
		t.Error("Select was called even though gh is already the credential helper")
	}
	if f.ShouldSetup() {
		t.Error("ShouldSetup() = true, want false when gh is already the credential helper")
	}
	if !slices.Contains(f.Scopes(), "workflow") {
		t.Errorf("Scopes() = %v, want it to include workflow", f.Scopes())
	}
}

func TestSetup_expiring_externalHelper_configuresOurs(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	// Configuring gh writes the helper config; no reject and no prompt are expected for an expiring login.
	cs.Register(`git config --global --replace-all credential\.`, 0, "")
	cs.Register(`git config --global --add credential\.`, 0, "", func(args []string) {
		if val := args[len(args)-1]; val != "!/path/to/gh auth git-credential" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})

	f := GitCredentialFlow{
		// Prompter is deliberately nil: Setup must not re-prompt for an expiring login.
		expiring: true,
		helper:   gitcredentials.Helper{Cmd: "osxkeychain"},
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
		t.Errorf("Setup() warning = %q, want empty after configuring gh for an expiring login", warning)
	}
}

func TestDetectConfiguredHelper(t *testing.T) {
	tests := []struct {
		name      string
		helper    gitcredentials.Helper
		helperErr error
		want      string
	}{
		{name: "ours", helper: gitcredentials.Helper{Cmd: "!/path/to/gh auth git-credential"}, want: "ours"},
		{name: "other", helper: gitcredentials.Helper{Cmd: "osxkeychain"}, want: "other"},
		{name: "none", helper: gitcredentials.Helper{}, want: "none"},
		{name: "error", helperErr: errors.New("git config failed"), want: "error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := GitCredentialFlow{
				HelperConfig: &stubHelperConfig{helper: tt.helper, helperErr: tt.helperErr},
			}
			if got := f.DetectConfiguredHelper("example.com"); got != tt.want {
				t.Errorf("DetectConfiguredHelper() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResultingHelperCategory_detectionErrorWithoutSetup(t *testing.T) {
	f := GitCredentialFlow{
		HelperConfig: &stubHelperConfig{helperErr: errors.New("git config failed")},
	}
	// A failed detection with no subsequent setup must stay "error", not collapse to "none".
	f.DetectConfiguredHelper("example.com")
	if got := f.ResultingHelperCategory(); got != "error" {
		t.Errorf("ResultingHelperCategory() = %q, want %q", got, "error")
	}
}
