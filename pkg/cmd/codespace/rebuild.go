package codespace

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

func newRebuildCmd(app *App) *cobra.Command {
	var codespace string

	rebuildCmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild a codespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Rebuild(cmd.Context(), codespace)
		},
	}

	rebuildCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")

	return rebuildCmd
}

func (a *App) Rebuild(ctx context.Context, codespaceName string) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	codespace, err := getOrChooseCodespace(ctx, a.apiClient, codespaceName)
	if err != nil {
		return err
	}

	// Users can't change their codespace while it's rebuilding, so there's
	// no need to execute this command.
	if codespace.State == api.CodespaceStateRebuilding {
		fmt.Fprintf(a.io.Out, "%s is already rebuilding\n", codespace.Name)
		return nil
	}

	session, err := startLiveShareSession(ctx, codespace, a, false, "")
	if err != nil {
		return err
	}
	defer safeClose(session, &err)

	err = session.Rebuild(ctx)
	if err != nil {
		return fmt.Errorf("couldn't rebuild codespace: %w", err)
	}

	fmt.Fprintf(a.io.Out, "%s is rebuilding\n", codespace.Name)
	return nil
}
