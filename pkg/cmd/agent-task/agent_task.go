package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/agent-task/create"
	cmdList "github.com/cli/cli/v2/pkg/cmd/agent-task/list"
	cmdView "github.com/cli/cli/v2/pkg/cmd/agent-task/view"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

// NewCmdAgentTask creates the base `agent-task` command.
func NewCmdAgentTask(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "agent-task <command>",
		Aliases: []string{"agent-tasks", "agent", "agents"},
		Short:   "Work with agent tasks (preview)",
		Long: heredoc.Doc(`
			Working with agent tasks in the GitHub CLI is in preview and
			subject to change without notice.
		`),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A task can be identified as argument in any of the following formats:
				- by pull request number, e.g. "123"; or
				- by session ID, e.g. "12345abc-12345-12345-12345-12345abc"; or
				- by URL, e.g. "https://github.com/OWNER/REPO/pull/123/agent-sessions/12345abc-12345-12345-12345-12345abc";

				Identifying tasks by pull request is not recommended for non-interactive use cases as
				there may be multiple tasks for a given pull request that require disambiguation.
			`),
		},
		Example: heredoc.Doc(`
			# List your most recent agent tasks
			$ gh agent-task list
			
			# Create a new agent task on the current repository
			$ gh agent-task create "Improve the performance of the data processing pipeline"
			
			# View details about agent tasks associated with a pull request
			$ gh agent-task view 123

			# View details about a specific agent task
			$ gh agent-task view 12345abc-12345-12345-12345-12345abc
		`),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return requireOAuthToken(f)
		},
		// This is required to run this root command. We want to
		// run it to test PersistentPreRunE behavior.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	// register subcommands
	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdView.NewCmdView(f, nil))

	return cmd
}

// requireOAuthToken ensures an OAuth (device flow) token is present and valid.
// agent-task subcommands inherit this check via PersistentPreRunE.
func requireOAuthToken(f *cmdutil.Factory) error {
	cfg, err := f.Config()
	if err != nil {
		return err
	}

	authCfg := cfg.Authentication()
	host, _ := authCfg.DefaultHost()
	if host == "" {
		return errors.New("no default host configured; run 'gh auth login'")
	}

	if auth.IsEnterprise(host) {
		return errors.New("agent tasks are not supported on this host")
	}

	token, source := authCfg.ActiveToken(host)

	// Tokens from sources "oauth_token" and "keyring" are likely
	// minted through our device flow.
	tokenSourceIsDeviceFlow := source == "oauth_token" || source == "keyring"
	// Tokens with "gho_" prefix are OAuth tokens.
	//
	// TODO: this matches a token prefix itself. It could ask
	// gh.AuthConfig.ActiveTokenType instead.
	tokenIsOAuth := strings.HasPrefix(token, "gho_")

	// Reject if the token is not from a device flow source or is not an OAuth token
	if !tokenSourceIsDeviceFlow || !tokenIsOAuth {
		return fmt.Errorf("this command requires an OAuth token. Re-authenticate with: gh auth login")
	}
	return nil
}
