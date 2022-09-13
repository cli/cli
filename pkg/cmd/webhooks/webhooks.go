package webhooks

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdWebhooks(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhooks <command>",
		Short: "Test a webhooks integration",
		Long:  `Create a test webhook, trigger and receive it locally`,
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(newCmdForward(f, nil))

	return cmd
}
