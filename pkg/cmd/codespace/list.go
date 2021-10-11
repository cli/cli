package codespace

import (
	"context"
	"fmt"
	"os"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	AsJSON       bool
	LimitResults int
}

func newListCmd(app *App) *cobra.Command {
	opts := &ListOptions{}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List your codespaces",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.LimitResults < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.LimitResults)}
			}

			return app.List(cmd.Context(), opts)
		},
	}

	listCmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	listCmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number of codespaces to fetch")

	return listCmd
}

func (a *App) List(ctx context.Context, opts *ListOptions) error {
	ctxWithLimits := context.WithValue(ctx, &api.ListDefaultLimitKey{}, opts.LimitResults)
	codespaces, err := a.apiClient.ListCodespaces(ctxWithLimits)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	table := output.NewTable(os.Stdout, opts.AsJSON)
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
