package codespace

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newSelectCmd(app *App) *cobra.Command {
	codeCmd := &cobra.Command{
		Use:   "select",
		Short: "Select a codespace",
		Hidden: true,
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Select(cmd.Context(), "")
		},
	}

	return codeCmd
}

func (a *App) Select(ctx context.Context, name string) error {
	codespace, err := getOrChooseCodespace(ctx, a.apiClient, name)
	if err != nil {
		return err
	}

	a.io.Out.Write([]byte(fmt.Sprintf("%s\n", codespace.Name)));

	return nil
}
