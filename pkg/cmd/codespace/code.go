package codespace

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func newCodeCmd(app *App) *cobra.Command {
	var (
		codespace   string
		useInsiders bool
		useWeb      bool
	)

	codeCmd := &cobra.Command{
		Use:   "code",
		Short: "Open a codespace in Visual Studio Code",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.VSCode(cmd.Context(), codespace, useInsiders, useWeb)
		},
	}

	codeCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	codeCmd.Flags().BoolVar(&useInsiders, "insiders", false, "Use the insiders version of Visual Studio Code")
	codeCmd.Flags().BoolVar(&useWeb, "web", false, "Use the web version of Visual Studio Code")

	return codeCmd
}

// VSCode opens a codespace in the local VS VSCode application.
func (a *App) VSCode(ctx context.Context, codespaceName string, useInsiders bool, useWeb bool) error {
	codespace, err := getOrChooseCodespace(ctx, a.apiClient, codespaceName)
	if err != nil {
		return err
	}

	if useWeb {
		if err := a.browser.Browse(codespace.WebUrl); err != nil {
			return fmt.Errorf("error opening Codespace: %w", err)
		}

		return nil
	}

	url := vscodeProtocolURL(codespace.Name, useInsiders)
	if err := a.browser.Browse(url); err != nil {
		return fmt.Errorf("error opening Visual Studio Code: %w", err)
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
