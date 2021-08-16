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

func NewCodeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "code [<codespace>]",
		Short: "Open a Codespace in VS Code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var codespaceName string
			if len(args) > 0 {
				codespaceName = args[0]
			}
			return Code(codespaceName)
		},
	}
}

func init() {
	rootCmd.AddCommand(NewCodeCmd())
}

func Code(codespaceName string) error {
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
				fmt.Println(err.Error())
				return nil
			}
			return fmt.Errorf("error choosing codespace: %v", err)
		}
		codespaceName = codespace.Name
	}

	if err := open.Run(vscodeProtocolURL(codespaceName)); err != nil {
		return fmt.Errorf("error opening vscode URL")
	}

	return nil
}

func vscodeProtocolURL(codespaceName string) string {
	return fmt.Sprintf("vscode://github.codespaces/connect?name=%s", url.QueryEscape(codespaceName))
}
