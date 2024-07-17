package sponsors

import (
	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdSponsors(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sponsors <command>",
		Short: "Manage sponsors",
		Long:  `Work with GitHub sponsors.`,
	}

	cmd.AddCommand(listcmd.NewCmdList(f, nil))
	return cmd
}
