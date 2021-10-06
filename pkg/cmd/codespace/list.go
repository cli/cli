package codespace

import (
	"context"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
	"github.com/spf13/cobra"
)

func newListCmd(app *App) *cobra.Command {
	var asJSON bool

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List your codespaces",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.List(cmd.Context(), asJSON)
		},
	}

	listCmd.Flags().BoolVar(&asJSON, "json", false, "Output as JSON")

	return listCmd
}

func (a *App) List(ctx context.Context, asJSON bool) error {
	codespaces, err := a.apiClient.ListCodespaces(ctx)
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
