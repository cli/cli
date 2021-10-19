package codespace

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/utils"
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

	if err := a.IO.StartPager(); err != nil {
		return fmt.Errorf("error starting pager: %v", err)
	}
	defer a.IO.StopPager()

	cs := a.IO.ColorScheme()
	tp := utils.NewTablePrinter(a.IO)

	for _, apiCodespace := range codespaces {
		c := codespace{apiCodespace}

		tp.AddField(c.Name, nil, nil)
		tp.AddField(c.RepositoryNWO, nil, cs.Bold)
		tp.AddField(c.branchWithGitStatus(), nil, nil)

		if c.Environment.State == api.CodespaceEnvironmentStateShutdown {
			tp.AddField(c.Environment.State, nil, cs.Gray)
		} else if c.Environment.State == api.CodespaceEnvironmentStateStarting {
			tp.AddField(c.Environment.State, nil, cs.Yellow)
		} else if c.Environment.State == api.CodespaceEnvironmentStateAvailable {
			tp.AddField(c.Environment.State, nil, cs.Green)
		} else {
			tp.AddField(c.Environment.State, nil, cs.Gray)
		}

		tp.AddField(c.CreatedAt, nil, cs.Gray)
		tp.EndRow()
	}

	if a.IO.IsStdoutTTY() {
		title := listHeader(len(codespaces))
		fmt.Printf("\n%s\n\n", title)
	}

	return tp.Render()
}

func listHeader(count int) string {
	s := ""
	if count == 0 {
		return "No results"
	} else if count > 1 {
		s = "s"
	}

	return fmt.Sprintf("Showing %d codespace%s", count, s)
}
