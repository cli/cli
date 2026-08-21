package field

import (
	cmdList "github.com/cli/cli/v2/pkg/cmd/issue/field/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdField creates the issue field command.
func NewCmdField(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "field <command>",
		Short: "View issue fields",
	}

	cmdutil.AddGroup(cmd, "General commands", cmdList.NewCmdList(f, nil))
	return cmd
}
