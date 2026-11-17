// Package event implements the gh event command set.
package event

import (
	"github.com/MakeNowJust/heredoc"
	cmdListen "github.com/cli/cli/v2/pkg/cmd/event/listen"
	cmdSubscription "github.com/cli/cli/v2/pkg/cmd/event/subscription"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdEvent(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event <command>",
		Short: "Listen to GitHub events (preview)",
		Long: heredoc.Docf(`
			Listen to events on GitHub resources as they happen, instead of polling. Changes made before
			the listener starts are not reported, so read the resource once it is listening.

			This is a preview, available on %[1]sgithub.com%[1]s to accounts enabled for event subscriptions.
		`, "`"),
	}

	cmd.AddCommand(cmdListen.NewCmdListen(f, nil))
	cmd.AddCommand(cmdSubscription.NewCmdSubscription(f))
	return cmd
}
