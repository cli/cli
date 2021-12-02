package codespace

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

func newCodeCmd(app *App) *cobra.Command {
	var (
		codespace   string
		useInsiders bool
	)

	codeCmd := &cobra.Command{
		Use:   "code",
		Short: "Open a codespace in Visual Studio Code",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := cmdutil.NewBrowser("", ioutil.Discard, app.io.ErrOut)
			return app.VSCode(cmd.Context(), b, codespace, useInsiders)
		},
	}

	codeCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	codeCmd.Flags().BoolVar(&useInsiders, "insiders", false, "Use the insiders version of Visual Studio Code")

	return codeCmd
}

// VSCode opens a codespace in the local VS VSCode application.
func (a *App) VSCode(ctx context.Context, browser browser, codespaceName string, useInsiders bool) error {
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
	if err := browser.Browse(url); err != nil {
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
