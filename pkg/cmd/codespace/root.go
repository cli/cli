package codespace

import (
	codespacesAPI "github.com/cli/cli/v2/internal/codespaces/api"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdCodespace(f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:     "codespace",
		Short:   "Connect to and manage codespaces",
		Aliases: []string{"cs"},
		GroupID: "core",
	}

	app := NewApp(
		f.IOStreams,
		f,
		codespacesAPI.New(f),
		f.Browser,
		f.Remotes,
	)

	root.AddCommand(newCodeCmd(app))
	root.AddCommand(newCreateCmd(app))
	root.AddCommand(newEditCmd(app))
	root.AddCommand(newDeleteCmd(app))
	root.AddCommand(newJupyterCmd(app))
	root.AddCommand(newListCmd(app))
	root.AddCommand(newViewCmd(app))
	root.AddCommand(newLogsCmd(app))
	root.AddCommand(newPortsCmd(app))
	root.AddCommand(newSSHCmd(app))
	root.AddCommand(newCpCmd(app))
	root.AddCommand(newStopCmd(app))
	root.AddCommand(newSelectCmd(app))
	root.AddCommand(newRebuildCmd(app))

	return root
}
