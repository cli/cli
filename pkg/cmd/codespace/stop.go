package codespace

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

func newStopCmd(app *App) *cobra.Command {
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

func (a *App) StopCodespace(ctx context.Context, codespaceName string) error {
	if codespaceName == "" {
		codespaces, err := a.apiClient.ListCodespaces(ctx, -1)
		if err != nil {
			return fmt.Errorf("failed to list codespace: %w", err)
		}

		var runningCodespaces []*api.Codespace
		for _, c := range codespaces {
			cs := codespace{c}
			if cs.isRunning() {
				runningCodespaces = append(runningCodespaces, c)
			}
		}
		if len(runningCodespaces) == 0 {
			return errors.New("no running codespaces")
		}

		codespace, err := chooseCodespaceFromList(ctx, runningCodespaces)
		if err != nil {
			return fmt.Errorf("failed to choose codespace: %w", err)
		}
		codespaceName = codespace.Name
	} else {
		c, err := a.apiClient.GetCodespace(ctx, codespaceName, false)
		if err != nil {
			return fmt.Errorf("failed to get codespace: %w", err)
		}
		cs := codespace{c}
		if !cs.isRunning() {
			return fmt.Errorf("codespace %q is not running", codespaceName)
		}
	}

	if err := a.apiClient.StopCodespace(ctx, codespaceName); err != nil {
		return fmt.Errorf("failed to stop codespace: %w", err)
	}
	a.logger.Println("Codespace stopped")

	return nil
}
