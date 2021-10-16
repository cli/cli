package codespace

import (
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

func newLogsCmd(app *codespaces.App) *cobra.Command {
	var (
		codespace string
		follow    bool
	)

	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Access codespace logs",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Logs(cmd.Context(), codespace, follow)
		},
	}

	logsCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Tail and follow the logs")

	return logsCmd
}
