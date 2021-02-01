package shared

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/run"
	"github.com/google/shlex"
)

func GitCredentialSetup(hostname, username, password, helper string) error {
	if helper == "" {
		// use GitHub CLI as a credential helper (for this host only)
		configureCmd, err := git.GitCommand("config", "--global", gitCredentialHelperKey(hostname), "!gh auth git-credential")
		if err != nil {
			return err
		}
		return run.PrepareCmd(configureCmd).Run()
	}

	// clear previous cached credentials
	rejectCmd, err := git.GitCommand("credential", "reject")
	if err != nil {
		return err
	}

	rejectCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
	`, hostname))

	err = run.PrepareCmd(rejectCmd).Run()
	if err != nil {
		return err
	}

	approveCmd, err := git.GitCommand("credential", "approve")
	if err != nil {
		return err
	}

	approveCmd.Stdin = bytes.NewBufferString(heredoc.Docf(`
		protocol=https
		host=%s
		username=%s
		password=%s
	`, hostname, username, password))

	err = run.PrepareCmd(approveCmd).Run()
	if err != nil {
		return err
	}

	return nil
}

func gitCredentialHelperKey(hostname string) string {
	return fmt.Sprintf("credential.https://%s.helper", hostname)
}

func gitCredentialHelper(hostname string) (helper string, err error) {
	helper, err = git.Config(gitCredentialHelperKey(hostname))
	if helper != "" {
		return
	}
	helper, err = git.Config("credential.helper")
	return
}

func isOurCredentialHelper(cmd string) bool {
	if !strings.HasPrefix(cmd, "!") {
		return false
	}

	args, err := shlex.Split(cmd[1:])
	if err != nil || len(args) == 0 {
		return false
	}

	return strings.TrimSuffix(filepath.Base(args[0]), ".exe") == "gh"
}
