package license

import (
	cmdList "github.com/cli/cli/v2/pkg/cmd/repo/license/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdLicense(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "license <command>",
		Short: "View available repository license templates",
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
