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

// Reject removes any stored credentials for a given hostname from the git credential helper.
func (u *Updater) Reject(hostname string) error {
	ctx := context.TODO()

	rejectCmd, err := u.GitClient.Command(ctx, "credential", "reject")
	if err != nil {
		return err
	}

	rejectCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
	`, hostname))

	_, err = rejectCmd.Output()
	return err
}

// Update updates the git credentials for a given hostname, first by rejecting any existing credentials and then
// approving the new credentials.
func (u *Updater) Update(hostname, username, password string) error {
	if err := u.Reject(hostname); err != nil {
		return err
	}

	ctx := context.TODO()

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
	return err
}
