package codespace

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "codespace",
		Short: "Connect to and manage codespaces",
	}

	root.AddCommand(newCodeCmd(app))
	root.AddCommand(newCreateCmd(app))
	root.AddCommand(newEditCmd(app))
	root.AddCommand(newDeleteCmd(app))
	root.AddCommand(newJupyterCmd(app))
	root.AddCommand(newListCmd(app))
	root.AddCommand(newLogsCmd(app))
	root.AddCommand(newPortsCmd(app))
	root.AddCommand(newSSHCmd(app))
	root.AddCommand(newCpCmd(app))
	root.AddCommand(newStopCmd(app))
	root.AddCommand(newSelectCmd(app))
	root.AddCommand(newRebuildCmd(app))

	return root
}
