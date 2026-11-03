package gitcredentials_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/stretchr/testify/require"
)

func configureStoreCredentialHelper(t *testing.T) {
	t.Helper()
	tmpCredentialsFile := filepath.Join(t.TempDir(), "credentials")

	gc := &git.Client{}
	// Use `--file` to store credentials in a temporary file that gets cleaned up when the test has finished running
	cmd, err := gc.Command(context.Background(), "config", "--global", "--add", "credential.helper", fmt.Sprintf("store --file %s", tmpCredentialsFile))
	require.NoError(t, err)
	require.NoError(t, cmd.Run())
}

func fillCredentials(t *testing.T) string {
	gc := &git.Client{}
	fillCmd, err := gc.Command(context.Background(), "credential", "fill")
	require.NoError(t, err)

	fillCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
	`, "github.com"))

	b, err := fillCmd.Output()
	require.NoError(t, err)

	return string(b)
}

func TestUpdateAddsNewCredentials(t *testing.T) {
	// Given we have an isolated git config and we're using the built in store credential helper
	// https://git-scm.com/docs/git-credential-store
	git.IsolateConfig(t)
	configureStoreCredentialHelper(t)

	// When we add new credentials
	u := &gitcredentials.Updater{
		GitClient: &git.Client{},
	}
	require.NoError(t, u.Update("github.com", "monalisa", "password"))

	// Then our credential description is successfully filled
	require.Equal(t, heredoc.Doc(`
protocol=https
host=github.com
username=monalisa
password=password
`), fillCredentials(t))
}

func TestUpdateReplacesOldCredentials(t *testing.T) {
	// Given we have an isolated git config and we're using the built in store credential helper
	// https://git-scm.com/docs/git-credential-store
	// and we have existing credentials
	git.IsolateConfig(t)
	configureStoreCredentialHelper(t)

	// When we replace old credentials
	u := &gitcredentials.Updater{
		GitClient: &git.Client{},
	}
	require.NoError(t, u.Update("github.com", "monalisa", "old-password"))
	require.NoError(t, u.Update("github.com", "monalisa", "new-password"))

	// Then our credential description is successfully filled
	require.Equal(t, heredoc.Doc(`
protocol=https
host=github.com
username=monalisa
password=new-password
`), fillCredentials(t))
}

func TestRejectExisting(t *testing.T) {
	cs, restoreRun := run.Stub()
	defer restoreRun(t)
	// Only reject is registered: approve must not be called, otherwise the stub panics.
	cs.Register(`git credential reject`, 0, "")

	u := &gitcredentials.Updater{
		GitClient: &git.Client{GitPath: "some/path/git"},
	}
	require.NoError(t, u.RejectExisting("github.com"))
}
