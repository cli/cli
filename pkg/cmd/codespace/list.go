package codespace

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	AsJSON bool
	Pretty bool
	Limit  int
}

func newListCmd(app *App) *cobra.Command {
	opts := ListOptions{}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List your codespaces",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.Limit)}
			}

			return app.List(cmd.Context(), &opts)
		},
	}

	listCmd.Flags().BoolVar(&opts.AsJSON, "json", false, "Output as JSON")
	listCmd.Flags().BoolVar(&opts.Pretty, "pretty", false, "Prettify JSON")
	listCmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of codespaces to list")

	return listCmd
}

func (a *App) List(ctx context.Context, opts *ListOptions) error {
	codespaces, err := a.apiClient.ListCodespaces(ctx, opts.Limit)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	if err := a.io.StartPager(); err != nil {
		return fmt.Errorf("error starting pager: %w", err)
	}
	defer a.io.StopPager()

	cs := a.io.ColorScheme()
	tp := utils.NewTablePrinterWithOptions(a.io, utils.TablePrinterOptions{
		IsTTY:      a.io.IsStdoutTTY(),
		IsJSON:     opts.AsJSON,
		JSONPretty: opts.Pretty,
	})

	tp.SetHeader([]string{"Name", "Repository", "Branch", "State", "Created at"})
	for _, apiCodespace := range codespaces {
		c := codespace{apiCodespace}

		tp.AddField(c.Name, nil, nil)
		tp.AddField(c.Repository.FullName, nil, cs.Bold)
		tp.AddField(c.branchWithGitStatus(), nil, nil)

		if c.State == api.CodespaceStateShutdown {
			tp.AddField(c.State, nil, cs.Gray)
		} else if c.State == api.CodespaceStateStarting {
			tp.AddField(c.State, nil, cs.Yellow)
		} else if c.State == api.CodespaceStateAvailable {
			tp.AddField(c.State, nil, cs.Green)
		} else {
			tp.AddField(c.State, nil, cs.Gray)
		}

		tp.AddField(c.CreatedAt, nil, cs.Gray)
		tp.EndRow()
	}

	if a.io.IsStdoutTTY() && !opts.AsJSON {
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
