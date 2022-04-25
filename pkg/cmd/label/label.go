package label

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdLabel(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label <command>",
		Short: "Manage labels",
		Long:  `Work with GitHub labels.`,
	}
	cmdutil.EnableRepoOverride(cmd, f)

	cmd.AddCommand(newCmdList(f, nil))
	cmd.AddCommand(newCmdCreate(f, nil))
	cmd.AddCommand(newCmdClone(f, nil))

	return cmd
}
