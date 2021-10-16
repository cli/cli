package codespaces

import (
	"context"
	"fmt"
	"net/url"

	"github.com/skratchdot/open-golang/open"
)

// VSCode opens a codespace in the local VS VSCode application.
func (a *App) VSCode(ctx context.Context, codespaceName string, useInsiders bool) error {
	if codespaceName == "" {
		codespace, err := a.chooseCodespace(ctx)
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
