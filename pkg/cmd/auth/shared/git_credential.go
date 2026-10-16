package shared

import (
	"errors"
	"fmt"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

type HelperConfig interface {
	ConfigureOurs(hostname string) error
	ConfiguredHelper(hostname string) (gitcredentials.Helper, error)
}

// PromptResult reports what happened when the git credential helper prompt was offered during a login or refresh.
// It is reported even when no prompt was shown, so callers can distinguish a declined or cancelled prompt from one
// that was short-circuited because gh is already the configured helper.
type PromptResult string

const (
	// PromptConfigure means the user chose to set up gh as the git credential helper.
	PromptConfigure PromptResult = "configure"
	// PromptDecline means the user chose not to set up gh as the git credential helper.
	PromptDecline PromptResult = "decline"
	// PromptCancel means the user cancelled the sign-in from the expiring credential helper prompt.
	PromptCancel PromptResult = "cancel"
	// PromptNotShown means no prompt was presented, for example because gh is already the configured helper.
	PromptNotShown PromptResult = "not_shown"
)

type GitCredentialFlow struct {
	Prompter Prompt

	HelperConfig HelperConfig
	Updater      *gitcredentials.Updater

	shouldSetup    bool
	expiring       bool
	helper         gitcredentials.Helper
	scopes         []string
	configuredOurs bool
}

// Prompt offers to set up gh as the git credential helper for hostname and reports the outcome as a PromptResult. The
// result is meaningful even when the returned error is non-nil (for example PromptCancel accompanies a cancellation),
// so callers can record what happened before handling the error. When gh is already the configured helper no prompt is
// shown and PromptNotShown is returned.
func (flow *GitCredentialFlow) Prompt(hostname string, expiring bool) (PromptResult, error) {
	flow.expiring = expiring

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
		return PromptNotShown, nil
	}

	// An expiring login needs gh as the credential helper so it can refresh the token, so we present a dedicated
	// prompt that spells out the consequence of declining and offers to stop the sign-in entirely.
	if expiring {
		return flow.promptExpiring(hostname, configuredHelperErr)
	}

	// Prompt the user for whether they want to configure git with the newly obtained token
	result, err := flow.Prompter.Confirm("Authenticate Git with your GitHub credentials?", true)
	if err != nil {
		return PromptNotShown, err
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
		if _, ok := errors.AsType[*git.NotInstalled](configuredHelperErr); ok {
			return PromptConfigure, configuredHelperErr
		}

		// On the other hand, if the user has requested setup we'll additionally request the workflow
		// scope to ensure that git push operations that include workflow changes succeed.
		flow.scopes = append(flow.scopes, "workflow")
		return PromptConfigure, nil
	}

	return PromptDecline, nil
}

// promptExpiring runs the credential-helper decision for an expiring login. gh must be the git credential helper to
// refresh an expiring token, so the user is told that declining will break their HTTPS git operations once the token
// expires and is offered three choices: configure gh, continue without it, or stop the sign-in. configuredHelperErr
// carries any error from detecting the existing helper so that a missing git binary aborts before the auth flow runs.
func (flow *GitCredentialFlow) promptExpiring(hostname string, configuredHelperErr error) (PromptResult, error) {
	options := []string{
		"Set up gh as your Git credential helper (recommended)",
		"Continue without it (you manage Git credentials yourself)",
		"Cancel",
	}

	message := "You asked for an expiring token, which gh refreshes automatically. For that to work, gh must be your " +
		"Git credential helper for " + hostname + ". Otherwise Git will not learn about refreshed tokens and your " +
		"HTTPS Git operations will start to fail once the current token expires, within hours. How would you like to proceed?"

	choice, err := flow.Prompter.Select(message, options[0], options)
	if err != nil {
		return PromptNotShown, err
	}

	switch choice {
	case 0: // Set up gh as the Git credential helper.
		// Abort before the auth flow if git isn't installed, matching the non-expiring prompt above.
		if _, ok := errors.AsType[*git.NotInstalled](configuredHelperErr); ok {
			return PromptConfigure, configuredHelperErr
		}
		flow.shouldSetup = true
		flow.scopes = append(flow.scopes, "workflow")
		return PromptConfigure, nil
	case 1: // Continue without gh as the Git credential helper.
		// shouldSetup stays false: the user accepted that git will not be configured with the new credential.
		return PromptDecline, nil
	case 2: // Cancel the sign-in.
		return PromptCancel, cmdutil.CancelError
	}

	return PromptNotShown, nil
}

func (flow *GitCredentialFlow) Scopes() []string {
	return flow.scopes
}

// DetectConfiguredHelper reads the git credential helper configured for hostname and returns its category ("ours",
// "other", or "none"). It performs its own read instead of relying on Prompt having run, so callers can observe the
// helper even for non-interactive or SSH sign-ins that never reach the prompt. The detected helper is stored on the
// flow so a later ResultingHelperCategory reflects it when no setup runs. The read is intentionally not cached: a
// repeated read inside Prompt stays cheap and keeps each caller's error handling independent.
func (flow *GitCredentialFlow) DetectConfiguredHelper(hostname string) string {
	flow.helper, _ = flow.HelperConfig.ConfiguredHelper(hostname)
	return helperCategory(flow.helper)
}

// ResultingHelperCategory reports the git credential helper in effect after Setup ran, as one of "ours", "other", or
// "none". It reflects the flow's own decisions without re-reading git configuration.
func (flow *GitCredentialFlow) ResultingHelperCategory() string {
	if flow.configuredOurs {
		return "ours"
	}
	return helperCategory(flow.helper)
}

func helperCategory(helper gitcredentials.Helper) string {
	switch {
	case helper.IsOurs():
		return "ours"
	case helper.IsConfigured():
		return "other"
	default:
		return "none"
	}
}

func (flow *GitCredentialFlow) ShouldSetup() bool {
	return flow.shouldSetup
}

// Setup configures git to use the newly obtained credential for the given hostname.
//
// If no credential helper is configured for the host, gh configures itself as the helper. Otherwise a non-gh helper
// is already configured and the behavior depends on how the login was started:
//
//   - For an expiring login the user already chose up-front (in Prompt) to make gh the helper, so gh is configured as
//     the helper without re-prompting and regardless of the token kind the server actually returned.
//   - For a non-expiring credential the existing helper is updated with the new credential.
//   - For a credential that unexpectedly came back refreshable, the external helper cannot refresh it and would keep
//     serving it after it expires, so Setup offers to configure gh as the helper instead. If the user accepts, gh is
//     configured as the helper. If the user declines, the existing credential is rejected and nothing is stored, and a
//     non-empty warning is returned so the caller can advise the user. The warning is undecorated so the caller
//     controls presentation.
//
// This is the earliest point where both facts are known: whether the token is refreshable is decided by the server
// during the auth flow, so the --expiring preference alone cannot determine it beforehand. Setup is only reached
// when the user already opted into git setup during the interactive prompt, so prompting again here is safe.
func (flow *GitCredentialFlow) Setup(hostname, username, authToken string, refreshable bool) (string, error) {
	// If there is no credential helper configured then we will set ourselves up as
	// the credential helper for this host.
	if !flow.helper.IsConfigured() {
		flow.configuredOurs = true
		return "", flow.HelperConfig.ConfigureOurs(hostname)
	}

	// For an expiring login the credential-helper decision was already made up-front in Prompt. Setup only runs when
	// the user chose to configure gh, so honor that choice here without re-prompting and regardless of whether the
	// server actually issued a refreshable token.
	if flow.expiring {
		flow.configuredOurs = true
		return "", flow.HelperConfig.ConfigureOurs(hostname)
	}

	// A non-gh helper is configured. It cannot refresh an expiring token and would keep serving the token after it
	// expires, so storing a refreshable credential in it is a trap. Offer to make gh the helper instead, since gh can
	// refresh the token transparently.
	if refreshable {
		setupGh, err := flow.Prompter.Confirm(
			fmt.Sprintf("The git credential helper configured for %s cannot refresh expiring tokens. Set up gh as the git credential helper instead?", hostname),
			true,
		)
		if err != nil {
			return "", err
		}
		if setupGh {
			flow.configuredOurs = true
			return "", flow.HelperConfig.ConfigureOurs(hostname)
		}

		// The user declined, so we clear any existing credential and store nothing, surfacing the problem immediately
		// rather than silently after expiry, and return a warning for the caller to show.
		if err := flow.Updater.RejectExisting(hostname); err != nil {
			return "", err
		}
		return fmt.Sprintf(
			"The configured git credential helper for %[1]s cannot refresh expiring tokens, so the new credential was not stored in it. Git operations may prompt for credentials or fail on the next attempt. Run 'gh auth setup-git --hostname %[1]s' to let gh manage git authentication.",
			hostname,
		), nil
	}

	// Otherwise, we'll tell git to inform the existing credential helper of the new credentials.
	return "", flow.Updater.Update(hostname, username, authToken)
}
