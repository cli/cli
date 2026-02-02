package dirs

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/tableprinter"
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
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tp := tableprinter.New(f.IOStreams, tableprinter.WithHeader("NAME", "PATH", "DESCRIPTION"))

			tp.AddField("CONFIG DIR", tableprinter.WithColor(f.IOStreams.ColorScheme().Bold))
			tp.AddField(config.ConfigDir())
			tp.AddField("Directory for storing configuration files")
			tp.EndRow()

			tp.AddField("DATA DIR", tableprinter.WithColor(f.IOStreams.ColorScheme().Bold))
			tp.AddField(config.DataDir())
			tp.AddField("Directory for storing downloaded data like extensions and copilot")
			tp.EndRow()

			tp.AddField("STATE DIR", tableprinter.WithColor(f.IOStreams.ColorScheme().Bold))
			tp.AddField(config.StateDir())
			tp.AddField("Directory for storing state files")
			tp.EndRow()

			return tp.Render()
		},
	}

	cmdutil.DisableAuthCheck(cmd)

	return cmd
}
