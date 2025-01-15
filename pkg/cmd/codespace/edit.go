package codespace

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type editOptions struct {
	selector    *CodespaceSelector
	displayName string
	machine     string
}

func newEditCmd(app *App) *cobra.Command {
	opts := editOptions{}

	editCmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.displayName == "" && opts.machine == "" {
				return cmdutil.FlagErrorf("must provide `--display-name` or `--machine`")
			}

			return app.Edit(cmd.Context(), opts)
		},
	}

	opts.selector = AddCodespaceSelector(editCmd, app.apiClient)
	editCmd.Flags().StringVarP(&opts.displayName, "display-name", "d", "", "Set the display name")
	editCmd.Flags().StringVar(&opts.displayName, "displayName", "", "display name")
	if err := editCmd.Flags().MarkDeprecated("displayName", "use `--display-name` instead"); err != nil {
		fmt.Fprintf(app.io.ErrOut, "error marking flag as deprecated: %v\n", err)
	}
	editCmd.Flags().StringVarP(&opts.machine, "machine", "m", "", "Set hardware specifications for the VM")

	return editCmd
}

// Edits a codespace
func (a *App) Edit(ctx context.Context, opts editOptions) error {
	codespaceName, err := opts.selector.SelectName(ctx)
	if err != nil {
		// TODO: is there a cleaner way to do this?
		if errors.Is(err, errNoCodespaces) || errors.Is(err, errNoFilteredCodespaces) {
			return err
		}
		return fmt.Errorf("error choosing codespace: %w", err)
	}

	err = a.RunWithProgress("Editing codespace", func() (err error) {
		_, err = a.apiClient.EditCodespace(ctx, codespaceName, &api.EditCodespaceParams{
			DisplayName: opts.displayName,
			Machine:     opts.machine,
		})
		return
	})
	if err != nil {
		return fmt.Errorf("error editing codespace: %w", err)
	}

	return nil
}
