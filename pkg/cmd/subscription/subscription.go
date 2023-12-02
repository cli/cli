package subscription

import (
	"github.com/MakeNowJust/heredoc"
	subGetCmd "github.com/cli/cli/v2/pkg/cmd/subscription/get"
	subListCmd "github.com/cli/cli/v2/pkg/cmd/subscription/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdSubscription(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "subscription <command>",
		Short:   "Manage subscriptions",
		Long:    "Work with GitHub subscriptions.",
		Aliases: []string{"sub"},
		Example: heredoc.Doc(`
			$ gh subscription list
			$ gh subscription get monalisa/hello-world
		`),
		GroupID: "core",
	}

	cmdutil.AddGroup(cmd, "General Commands",
		subListCmd.NewCmdList(f, nil),
		subGetCmd.NewCmdGet(f, nil),
	)

	return cmd
}
