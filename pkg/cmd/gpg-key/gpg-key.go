package key

import (
	cmdAdd "github.com/cli/cli/v2/pkg/cmd/gpg-key/add"
	cmdList "github.com/cli/cli/v2/pkg/cmd/gpg-key/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdGPGKey(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gpg-key <command>",
		Short: "Manage GPG keys",
		Long:  "Manage GPG keys registered with your GitHub account",
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))
	cmd.AddCommand(cmdAdd.NewCmdAdd(f, nil))

	return cmd
}
