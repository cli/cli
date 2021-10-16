package codespace

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "codespace",
		SilenceUsage:  true,  // don't print usage message after each error (see #80)
		SilenceErrors: false, // print errors automatically so that main need not
		Short:         "List, create, delete and SSH into codespaces",
		Long:          `Work with GitHub codespaces`,
		Example: heredoc.Doc(`
			$ gh codespace list
			$ gh codespace create
			$ gh codespace delete
			$ gh codespace ssh
		`),
	}

	root.AddCommand(newCodeCmd(app))
	root.AddCommand(newCreateCmd(app))
	root.AddCommand(newDeleteCmd(app))
	root.AddCommand(newListCmd(app))
	root.AddCommand(newLogsCmd(app))
	root.AddCommand(newPortsCmd(app))
	root.AddCommand(newSSHCmd(app))
	root.AddCommand(newStopCmd(app))

	return root
}
