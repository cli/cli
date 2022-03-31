package codespace

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

func newRenameCmd(app *App) *cobra.Command {
	var codespace string
	var newName string

	renameCmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RenameCodespace(cmd.Context(), codespace, newName)
		},
	}
	renameCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	renameCmd.Flags().StringVarP(&newName, "display-name", "n", "", "New display name for the codespace")

	return renameCmd
}

func (a *App) RenameCodespace(ctx context.Context, codespaceName string, newName string) error {
	codespace, err := getOrChooseCodespace(ctx, a.apiClient, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	if err := a.apiClient.RenameCodespace(ctx, codespaceName, newName); err != nil {
		return fmt.Errorf("failed to rename codespace: %w", err)
	}

	return nil
}
