package shared

import (
	"fmt"
	"testing"

	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/prompt"
)

type tinyConfig map[string]string

func (c tinyConfig) Get(host, key string) (string, error) {
	return c[fmt.Sprintf("%s:%s", host, key)], nil
}

func TestGitCredentialSetup_configureExisting(t *testing.T) {
	cfg := tinyConfig{"example.com:oauth_token": "OTOKEN"}

	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config credential\.https://example\.com\.helper`, 1, "")
	cs.Register(`git config credential\.helper`, 0, "osxkeychain\n")
	cs.Register(`git credential reject`, 0, "")
	cs.Register(`git credential approve`, 0, "")

	as, restoreAsk := prompt.InitAskStubber()
	defer restoreAsk()
	as.StubOne(true)

	if err := GitCredentialSetup(cfg, "example.com", "monalisa"); err != nil {
		t.Errorf("GitCredentialSetup() error = %v", err)
	}
}

func TestGitCredentialSetup_setOurs(t *testing.T) {
	cfg := tinyConfig{"example.com:oauth_token": "OTOKEN"}

	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config credential\.https://example\.com\.helper`, 1, "")
	cs.Register(`git config credential\.helper`, 1, "")
	cs.Register(`git config --global credential\.https://example\.com\.helper`, 0, "", func(args []string) {
		if val := args[len(args)-1]; val != "!gh auth git-credential" {
			t.Errorf("global credential helper configured to %q", val)
		}
	})

	as, restoreAsk := prompt.InitAskStubber()
	defer restoreAsk()
	as.StubOne(true)

	if err := GitCredentialSetup(cfg, "example.com", "monalisa"); err != nil {
		t.Errorf("GitCredentialSetup() error = %v", err)
	}
}

func TestGitCredentialSetup_promptDeny(t *testing.T) {
	cfg := tinyConfig{"example.com:oauth_token": "OTOKEN"}

	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config credential\.https://example\.com\.helper`, 1, "")
	cs.Register(`git config credential\.helper`, 1, "")

	as, restoreAsk := prompt.InitAskStubber()
	defer restoreAsk()
	as.StubOne(false)

	if err := GitCredentialSetup(cfg, "example.com", "monalisa"); err != nil {
		t.Errorf("GitCredentialSetup() error = %v", err)
	}
}

func TestGitCredentialSetup_isOurs(t *testing.T) {
	cfg := tinyConfig{"example.com:oauth_token": "OTOKEN"}

	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	cs.Register(`git config credential\.https://example\.com\.helper`, 0, "!/path/to/gh auth\n")

	_, restoreAsk := prompt.InitAskStubber()
	defer restoreAsk()

	if err := GitCredentialSetup(cfg, "example.com", "monalisa"); err != nil {
		t.Errorf("GitCredentialSetup() error = %v", err)
	}
}
