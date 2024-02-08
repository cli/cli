package deployment

import (
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/deployment/create"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdDeployment(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment <command>",
		Short: "Manage GitHub deployments",
		Long:  `Work with GitHub deployments.`,
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdCreate.NewCmdCreate(f, nil))

	return cmd
}
