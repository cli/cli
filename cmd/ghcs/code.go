package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

func newCodeCmd() *cobra.Command {
	useInsiders := false

	codeCmd := &cobra.Command{
		Use:   "code [<codespace>]",
		Short: "Open a Codespace in VS Code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var codespaceName string
			if len(args) > 0 {
				codespaceName = args[0]
			}
			return code(codespaceName, useInsiders)
		},
	}

	codeCmd.Flags().BoolVar(&useInsiders, "insiders", false, "Use the insiders version of VS Code")

	return codeCmd
}

func init() {
	rootCmd.AddCommand(newCodeCmd())
}

func code(codespaceName string, useInsiders bool) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	if codespaceName == "" {
		codespace, err := codespaces.ChooseCodespace(ctx, apiClient, user)
		if err != nil {
			if err == codespaces.ErrNoCodespaces {
				return err
			}
			return fmt.Errorf("error choosing codespace: %v", err)
		}
		codespaceName = codespace.Name
	}

	if err := open.Run(vscodeProtocolURL(codespaceName, useInsiders)); err != nil {
		return fmt.Errorf("error opening vscode URL")
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
