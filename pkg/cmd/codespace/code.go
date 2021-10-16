package codespace

import (
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/spf13/cobra"
)

func newCodeCmd(app *codespaces.App) *cobra.Command {
	var (
		codespace   string
		useInsiders bool
	)

	codeCmd := &cobra.Command{
		Use:   "code",
		Short: "Open a codespace in VS Code",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.VSCode(cmd.Context(), codespace, useInsiders)
		},
	}

	codeCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	codeCmd.Flags().BoolVar(&useInsiders, "insiders", false, "Use the insiders version of VS Code")

	return codeCmd
}
