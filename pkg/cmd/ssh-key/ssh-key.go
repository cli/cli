package key

import (
	cmdList "github.com/cli/cli/pkg/cmd/ssh-key/list"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdSSHKey creates a command for manage SSH Keys
func NewCmdSSHKey(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-key <command>",
		Short: "Manage SSH keys",
		Long:  "Work with GitHub SSH keys",

		Hidden: true,
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
