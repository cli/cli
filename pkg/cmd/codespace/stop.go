package codespace

import (
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

func newStopCmd(app *codespaces.App) *cobra.Command {
	var codespace string

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop a running codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.StopCodespace(cmd.Context(), codespace)
		},
	}
	stopCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")

	return stopCmd
}
