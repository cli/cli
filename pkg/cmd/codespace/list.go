package codespace

import (
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func newListCmd(app *codespaces.App) *cobra.Command {
	var (
		asJSON bool
		limit  int
	)

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List your codespaces",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", limit)}
			}

			return app.List(cmd.Context(), asJSON, limit)
		},
	}

	listCmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")
	listCmd.Flags().IntVarP(&limit, "limit", "L", 30, "Maximum number of codespaces to list")

	return listCmd
}
