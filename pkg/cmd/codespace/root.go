package codespace

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewRootCmd(app *App, f *cmdutil.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:   "codespace",
		Short: "Connect to and manage your codespaces",
	}

	root.AddCommand(newCodeCmd(app))
	root.AddCommand(newCreateCmd(app))
	root.AddCommand(newDeleteCmd(app))
	root.AddCommand(newListCmd(app))
	root.AddCommand(newLogsCmd(app))
	root.AddCommand(newPortsCmd(app))
	root.AddCommand(newSSHCmd(app, f))
	root.AddCommand(newCpCmd(app))
	root.AddCommand(newStopCmd(app))

	return root
}
