package codespace

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/rpc"
	"github.com/spf13/cobra"
)

func newRebuildCmd(app *App) *cobra.Command {
	var (
		selector    *CodespaceSelector
		fullRebuild bool
	)

	rebuildCmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild a codespace",
		Long: `Rebuilding recreates your codespace. Your code and any current changes will be
preserved. Your codespace will be rebuilt using your working directory's
dev container. A full rebuild also removes cached Docker images.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Rebuild(cmd.Context(), selector, fullRebuild)
		},
	}

	selector = AddCodespaceSelector(rebuildCmd, app.apiClient)

	rebuildCmd.Flags().BoolVar(&fullRebuild, "full", false, "perform a full rebuild")

	return rebuildCmd
}

func (a *App) Rebuild(ctx context.Context, selector *CodespaceSelector, full bool) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	codespace, err := selector.Select(ctx)
	if err != nil {
		return err
	}

	// There's no need to rebuild again because users can't modify their codespace while it rebuilds
	if codespace.State == api.CodespaceStateRebuilding {
		fmt.Fprintf(a.io.Out, "%s is already rebuilding\n", codespace.Name)
		return nil
	}

	session, err := startLiveShareSession(ctx, codespace, a, false, "")
	if err != nil {
		return fmt.Errorf("starting Live Share session: %w", err)
	}
	defer safeClose(session, &err)

	invoker, err := rpc.CreateInvoker(ctx, session)
	if err != nil {
		return err
	}
	defer safeClose(invoker, &err)

	err = invoker.RebuildContainer(ctx, full)
	if err != nil {
		return fmt.Errorf("rebuilding codespace via session: %w", err)
	}

	fmt.Fprintf(a.io.Out, "%s is rebuilding\n", codespace.Name)
	return nil
}
