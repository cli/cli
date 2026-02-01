package dirs

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdDirs(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dirs",
		Short: "Print the directories gh uses for configuration, data, and state",
		Long: heredoc.Doc(`
			Print the directories that gh uses for storing configuration files,
			downloaded data, and state information.

			CONFIG DIR    Directory for storing configuration files
			DATA DIR      Directory for storing downloaded data like extensions and copilot
			STATE DIR     Directory for storing state files
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(f.IOStreams.Out, "CONFIG DIR    %s    Directory for storing configuration files\n", config.ConfigDir())
			fmt.Fprintf(f.IOStreams.Out, "DATA DIR      %s    Directory for storing downloaded data like extensions and copilot\n", config.DataDir())
			fmt.Fprintf(f.IOStreams.Out, "STATE DIR     %s    Directory for storing state files\n", config.StateDir())
			return nil
		},
	}

	cmdutil.DisableAuthCheck(cmd)

	return cmd
}
