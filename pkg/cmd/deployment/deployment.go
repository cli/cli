package deployment

import (
	cmdCreate "github.com/cli/cli/v2/pkg/cmd/deployment/create"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/deployment/delete"
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
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))

	return cmd
}
