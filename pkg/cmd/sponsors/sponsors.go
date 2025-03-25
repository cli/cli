package sponsorsß

import (
	cmdList "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdSponsors(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sponsors <command>",
		Short: "Manage sponsors",
	}

	cmd.AddCommand(cmdList.NewCmdList(f, nil))

	return cmd
}
