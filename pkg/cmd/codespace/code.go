package ghcs

import (
	"context"
	"fmt"
	"net/url"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

func newCodeCmd(app *App) *cobra.Command {
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

// VSCode opens a codespace in the local VS VSCode application.
func (a *App) VSCode(ctx context.Context, codespaceName string, useInsiders bool) error {
	if codespaceName == "" {
		codespace, err := chooseCodespace(ctx, a.apiClient)
		if err != nil {
			if err == errNoCodespaces {
				return err
			}
			return fmt.Errorf("error choosing codespace: %w", err)
		}
		codespaceName = codespace.Name
	}

	url := vscodeProtocolURL(codespaceName, useInsiders)
	if err := open.Run(url); err != nil {
		return fmt.Errorf("error opening vscode URL %s: %s. (Is VS Code installed?)", url, err)
	}

	return nil
}

func vscodeProtocolURL(codespaceName string, useInsiders bool) string {
	application := "vscode"
	if useInsiders {
		application = "vscode-insiders"
	}
	return fmt.Sprintf("%s://github.codespaces/connect?name=%s", application, url.QueryEscape(codespaceName))
}
