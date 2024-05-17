package shared

import (
	"errors"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
)

type HelperConfig interface {
	ConfigureOurs(hostname string) error
	ConfiguredHelper(hostname string) (gitcredentials.Helper, error)
}

type GitCredentialFlow struct {
	Prompter Prompt

	HelperConfig HelperConfig
	Updater      *gitcredentials.Updater

	shouldSetup bool
	helper      gitcredentials.Helper
	scopes      []string
}

func (flow *GitCredentialFlow) Prompt(hostname string) error {
	// First we'll fetch the credential helper that would be used for this host
	var configuredHelperErr error
	flow.helper, configuredHelperErr = flow.HelperConfig.ConfiguredHelper(hostname)
	// If the helper is gh itself, then we don't need to ask the user if they want to update their git credentials
	// because it will happen automatically by virtue of the fact that gh will return the active token.
	//
	// Since gh is the helper, this token may be used for git operations, so we'll additionally request the workflow
	// scope to ensure that git push operations that include workflow changes succeed.
	if flow.helper.IsOurs() {
		flow.scopes = append(flow.scopes, "workflow")
		return nil
	}

	// Prompt the user for whether they want to configure git with the newly obtained token
	result, err := flow.Prompter.Confirm("Authenticate Git with your GitHub credentials?", true)
	if err != nil {
		return err
	}
	flow.shouldSetup = result

	if flow.shouldSetup {
		// If the user does want to configure git, we'll check the error returned from fetching the configured helper
		// above. If the error indicates that git isn't installed, we'll return an error now to ensure that the auth
		// flow is aborted before the user goes any further.
		//
		// Note that this is _slightly_ naive because there may be other reasons that fetching the configured helper
		// fails that might cause later failures but this code has existed for a long time and I don't want to change
		// it as part of a refactoring.
		//
		// Refs:
		//  * https://git-scm.com/docs/git-config#_description
		//  * https://github.com/cli/cli/pull/4109
		var errNotInstalled *git.NotInstalled
		if errors.As(configuredHelperErr, &errNotInstalled) {
			return configuredHelperErr
		}

		// On the other hand, if the user has requested setup we'll additionally request the workflow
		// scope to ensure that git push operations that include workflow changes succeed.
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
	// If there is no credential helper configured then we will set ourselves up as
	// the credential helper for this host.
	if !flow.helper.IsConfigured() {
		return flow.HelperConfig.ConfigureOurs(hostname)
	}

	// Otherwise, we'll tell git to inform the existing credential helper of the new credentials.
	return flow.Updater.Update(hostname, username, authToken)
}
