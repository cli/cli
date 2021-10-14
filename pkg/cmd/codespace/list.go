package codespace

import (
	"context"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func newListCmd(app *App) *cobra.Command {
	var asJSON bool
	var limit int

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

func (a *App) List(ctx context.Context, asJSON bool, limit int) error {
	codespaces, err := a.apiClient.ListCodespaces(ctx, limit)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	table := output.NewTable(os.Stdout, asJSON)
	table.SetHeader([]string{"Name", "Repository", "Branch", "State", "Created At"})
	for _, apiCodespace := range codespaces {
		cs := codespace{apiCodespace}
		table.Append([]string{
			cs.Name,
			cs.RepositoryNWO,
			cs.branchWithGitStatus(),
			cs.Environment.State,
			cs.CreatedAt,
		})
	}

	table.Render()
	return nil
}
