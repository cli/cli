package shared

import (
	"errors"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
)

type GitCredentialFlow struct {
	Prompter Prompt

	HelperConfig *gitcredentials.HelperConfig
	Updater      *gitcredentials.Updater

	shouldSetup bool
	helper      gitcredentials.Helper
	scopes      []string
}

func (flow *GitCredentialFlow) Prompt(hostname string) error {
	var configuredHelperErr error
	flow.helper, configuredHelperErr = flow.HelperConfig.ConfiguredHelper(hostname)
	if flow.helper.IsOurs() {
		flow.scopes = append(flow.scopes, "workflow")
		return nil
	}

	result, err := flow.Prompter.Confirm("Authenticate Git with your GitHub credentials?", true)
	if err != nil {
		return err
	}
	flow.shouldSetup = result

	if flow.shouldSetup {
		var errNotInstalled *git.NotInstalled
		if errors.As(err, &errNotInstalled) {
			return configuredHelperErr
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
	if !flow.helper.IsConfigured() {
		return flow.HelperConfig.Configure(hostname)
	}

	// Otherwise, we'll tell git to inform the existing credential helper of the new credentials.
	return flow.Updater.Update(hostname, username, password)
}
