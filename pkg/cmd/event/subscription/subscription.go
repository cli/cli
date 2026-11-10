// Package subscription implements the gh event subscription command set.
package subscription

import (
	"github.com/MakeNowJust/heredoc"
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/event/subscription/create"
	cmdList "github.com/cli/cli/v2/pkg/cmd/event/subscription/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdSubscription(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "subscription <command>",
		Short: "Manage event subscriptions (preview)",
		Long: heredoc.Docf(`
			Manage the event subscriptions of your account.

			A subscription selects the event types to deliver for one resource, and %[1]sgh event listen%[1]s
			receives its events. Subscriptions expire after 24 hours, and each account can have a limited
			number of them at a time.
		`, "`"),
	}

	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))
	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	return cmd
}
