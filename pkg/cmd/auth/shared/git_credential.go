package shared

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/google/shlex"
)

type GitCredentialFlow struct {
	Prompter  Prompt
	GitClient *git.Client

	HelperConfigurer *gitcredentials.HelperConfigurer
	Updater          *gitcredentials.Updater

	shouldSetup bool
	helper      string
	scopes      []string
}

func (flow *GitCredentialFlow) Prompt(hostname string) error {
	var gitErr error
	flow.helper, gitErr = gitCredentialHelper(flow.GitClient, hostname)
	if isOurCredentialHelper(flow.helper) {
		flow.scopes = append(flow.scopes, "workflow")
		return nil
	}

	result, err := flow.Prompter.Confirm("Authenticate Git with your GitHub credentials?", true)
	if err != nil {
		return err
	}
	flow.shouldSetup = result

	if flow.shouldSetup {
		if isGitMissing(gitErr) {
			return gitErr
		}
		flow.scopes = append(flow.scopes, "workflow")
	}

	return nil
}

func (flow *GitCredentialFlow) Scopes() []string {
	return flow.scopes
}

func (flow *GitCredentialFlow) ShouldSetup() bool {
	return flow.shouldSetup
}

func (flow *GitCredentialFlow) Setup(hostname, username, authToken string) error {
	return flow.gitCredentialSetup(hostname, username, authToken)
}

func (flow *GitCredentialFlow) gitCredentialSetup(hostname, username, password string) error {
	// If there is no credential helper configured then we will set ourselves up as
	// the credential helper for this host.
	if flow.helper == "" {
		return flow.HelperConfigurer.Configure(hostname)
	}

	// Otherwise, we'll tell git to inform the existing credential helper of the new credentials.
	return flow.Updater.Update(hostname, username, password)
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

func isGitMissing(err error) bool {
	if err == nil {
		return false
	}
	var errNotInstalled *git.NotInstalled
	return errors.As(err, &errNotInstalled)
}

func gitCredentialHelperKey(hostname string) string {
	host := strings.TrimSuffix(ghinstance.HostPrefix(hostname), "/")
	return fmt.Sprintf("credential.%s.helper", host)
}

func gitCredentialHelper(gitClient *git.Client, hostname string) (helper string, err error) {
	ctx := context.TODO()

	helper, err = gitClient.Config(ctx, gitCredentialHelperKey(hostname))
	if helper != "" {
		return
	}
	helper, err = gitClient.Config(ctx, "credential.helper")
	return
}
