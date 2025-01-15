package deploykey

import (
	cmdAdd "github.com/cli/cli/v2/pkg/cmd/repo/deploy-key/add"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/repo/deploy-key/delete"
	cmdList "github.com/cli/cli/v2/pkg/cmd/repo/deploy-key/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdDeployKey(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy-key <command>",
		Short: "Manage deploy keys in a repository",
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdAdd.NewCmdAdd(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))

	return cmd
}
