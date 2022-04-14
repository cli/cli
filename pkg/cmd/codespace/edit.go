package codespace

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

type editOptions struct {
	codespaceName string
	displayName   string
	machine       string
}

func newEditCmd(app *App) *cobra.Command {
	opts := editOptions{}

	editCmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Edit(cmd.Context(), opts)
		},
	}

	editCmd.Flags().StringVarP(&opts.codespaceName, "codespace", "c", "", "Name of the codespace")
	editCmd.Flags().StringVarP(&opts.displayName, "displayName", "d", "", "display name")
	editCmd.Flags().StringVarP(&opts.machine, "machine", "m", "", "hardware specifications for the VM")

	return editCmd
}

// Edits a codespace
func (a *App) Edit(ctx context.Context, opts editOptions) error {
	userInputs := struct {
		CodespaceName string
		DisplayName   string
		SKU           string
	}{
		CodespaceName: opts.codespaceName,
		DisplayName:   opts.displayName,
		SKU:           opts.machine,
	}
	a.StartProgressIndicatorWithLabel("Editing codespace")
	_, err := a.apiClient.EditCodespace(ctx, userInputs.CodespaceName, &api.EditCodespaceParams{
		DisplayName: userInputs.DisplayName,
		Machine:     userInputs.SKU,
	})
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error editing codespace: %w", err)
	}

	return nil
}
