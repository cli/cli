package codespace

import (
	"context"
	"fmt"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

func newListCmd(app *App) *cobra.Command {
	var limit int
	var exporter cmdutil.Exporter

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List your codespaces",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", limit)
			}

			return app.List(cmd.Context(), limit, exporter)
		},
	}

	listCmd.Flags().IntVarP(&limit, "limit", "L", 30, "Maximum number of codespaces to list")
	cmdutil.AddJSONFlags(listCmd, &exporter, api.CodespaceFields)

	return listCmd
}

func (a *App) List(ctx context.Context, limit int, exporter cmdutil.Exporter) error {
	a.StartProgressIndicatorWithLabel("Fetching codespaces")
	codespaces, err := a.apiClient.ListCodespaces(ctx, limit)
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	if err := a.io.StartPager(); err != nil {
		a.errLogger.Printf("error starting pager: %v", err)
	}
	defer a.io.StopPager()

	if exporter != nil {
		return exporter.Write(a.io, codespaces)
	}

	tp := utils.NewTablePrinter(a.io)
	if tp.IsTTY() {
		tp.AddField("NAME", nil, nil)
		tp.AddField("REPOSITORY", nil, nil)
		tp.AddField("BRANCH", nil, nil)
		tp.AddField("STATE", nil, nil)
		tp.AddField("CREATED AT", nil, nil)
		tp.EndRow()
	}

	cs := a.io.ColorScheme()
	for _, apiCodespace := range codespaces {
		c := codespace{apiCodespace}

		var stateColor func(string) string
		switch c.State {
		case api.CodespaceStateStarting:
			stateColor = cs.Yellow
		case api.CodespaceStateAvailable:
			stateColor = cs.Green
		}

		tp.AddField(c.Name, nil, cs.Yellow)
		tp.AddField(c.Repository.FullName, nil, nil)
		tp.AddField(c.branchWithGitStatus(), nil, cs.Cyan)
		tp.AddField(c.State, nil, stateColor)

		if tp.IsTTY() {
			ct, err := time.Parse(time.RFC3339, c.CreatedAt)
			if err != nil {
				return fmt.Errorf("error parsing date %q: %w", c.CreatedAt, err)
			}
			tp.AddField(utils.FuzzyAgoAbbr(time.Now(), ct), nil, cs.Gray)
		} else {
			tp.AddField(c.CreatedAt, nil, nil)
		}
		tp.EndRow()
	}

	return tp.Render()
}
