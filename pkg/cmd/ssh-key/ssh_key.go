package key

import (
	cmdAdd "github.com/cli/cli/v2/pkg/cmd/ssh-key/add"
	cmdDelete "github.com/cli/cli/v2/pkg/cmd/ssh-key/delete"
	cmdList "github.com/cli/cli/v2/pkg/cmd/ssh-key/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdSSHKey(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-key <command>",
		Short: "Manage SSH keys",
		Long:  "Manage SSH keys registered with your GitHub account.",
	}

	cmd.AddCommand(cmdAdd.NewCmdAdd(f, nil))
	cmd.AddCommand(cmdDelete.NewCmdDelete(f, nil))
	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
