package gitcredentials_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/stretchr/testify/require"
)

func configureStoreCredentialHelper(t *testing.T) string {
	t.Helper()
	tmpCredentialsFile := filepath.Join(t.TempDir(), "credentials")

	gc := &git.Client{}
	// Use `--file` to store credentials in a temporary file that gets cleaned up when the test has finished running
	cmd, err := gc.Command(context.Background(), "config", "--global", "--add", "credential.helper", fmt.Sprintf("store --file %s", tmpCredentialsFile))
	require.NoError(t, err)
	require.NoError(t, cmd.Run())
	return tmpCredentialsFile
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

func TestRejectRemovesOnlyMatchingCredentials(t *testing.T) {
	git.IsolateConfig(t)
	credentialsFile := configureStoreCredentialHelper(t)

	u := &gitcredentials.Updater{GitClient: &git.Client{}}
	require.NoError(t, u.Update("github.com", "monalisa", "password"))

	require.NoError(t, u.Reject("github.com", "octocat"))
	contents, err := os.ReadFile(credentialsFile)
	require.NoError(t, err)
	require.NotEmpty(t, contents)

	require.NoError(t, u.Reject("github.com", "monalisa"))
	contents, err = os.ReadFile(credentialsFile)
	require.NoError(t, err)
	require.Empty(t, contents)
}
