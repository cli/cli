package ghcs

import (
	"context"
	"fmt"
	"os"

	"github.com/cli/cli/v2/cmd/ghcs/output"
	"github.com/cli/cli/v2/internal/codespaces/api"
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
	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespaces, err := a.apiClient.ListCodespaces(ctx, user.Login)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	table := output.NewTable(os.Stdout, asJSON)
	table.SetHeader([]string{"Name", "Repository", "Branch", "State", "Created At"})
	for _, codespace := range codespaces {
		table.Append([]string{
			codespace.Name,
			codespace.RepositoryNWO,
			codespace.Branch + dirtyStar(codespace.Environment.GitStatus),
			codespace.Environment.State,
			codespace.CreatedAt,
		})
	}

	table.Render()
	return nil
}

func dirtyStar(status api.CodespaceEnvironmentGitStatus) string {
	if status.HasUncommitedChanges || status.HasUnpushedChanges {
		return "*"
	}

	return ""
}
