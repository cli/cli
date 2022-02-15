package codespace

import (
	"context"
	"fmt"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

type editOptions struct {
	codespaceName string
	displayName   string
	idleTimeout   time.Duration
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
	editCmd.Flags().DurationVar(&opts.idleTimeout, "idle-timeout", 0, "allowed inactivity before codespace is stopped, e.g. \"10m\", \"1h\"")
	editCmd.Flags().StringVarP(&opts.machine, "machine", "m", "", "hardware specifications for the VM")

	return editCmd
}

// Edits a codespace
func (a *App) Edit(ctx context.Context, opts editOptions) error {
	userInputs := struct {
		CodespaceName string
		DisplayName   string
		IdleTimeout   time.Duration
		SKU           string
	}{
		CodespaceName: opts.codespaceName,
		DisplayName:   opts.displayName,
		IdleTimeout:   opts.idleTimeout,
		SKU:           opts.machine,
	}
	a.StartProgressIndicatorWithLabel("Editing codespace")
	codespace, err := a.apiClient.EditCodespace(ctx, userInputs.CodespaceName, &api.EditCodespaceParams{
		DisplayName:        userInputs.DisplayName,
		IdleTimeoutMinutes: int(userInputs.IdleTimeout.Minutes()),
		Machine:            userInputs.SKU,
	})
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error editing codespace: %w", err)
	}

	fmt.Fprintln(a.io.Out, codespace.Name)
	return nil
}
