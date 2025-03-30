package gitcredentials

import (
	"bytes"
	"context"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
)

// An Updater is used to update the git credentials for a given hostname.
type Updater struct {
	GitClient *git.Client
}

// Update updates the git credentials for a given hostname, first by rejecting any existing credentials and then
// approving the new credentials.
func (u *Updater) Update(hostname, username, password string) error {
	ctx := context.TODO()

	// clear previous cached credentials
	rejectCmd, err := u.GitClient.Command(ctx, "credential", "reject")
	if err != nil {
		return err
	}

	rejectCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
	`, hostname))

	_, err = rejectCmd.Output()
	if err != nil {
		return err
	}

	approveCmd, err := u.GitClient.Command(ctx, "credential", "approve")
	if err != nil {
		return err
	}

	approveCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
		username=%s
		password=%s
	`, hostname, username, password))

	_, err = approveCmd.Output()
	if err != nil {
		return err
	}

	return nil
}
