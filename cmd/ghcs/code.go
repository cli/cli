package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
)

func newCodeCmd() *cobra.Command {
	var (
		codespace   string
		useInsiders bool
	)

	log := output.NewLogger(os.Stdout, os.Stderr, false)

	codeCmd := &cobra.Command{
		Use:   "code",
		Short: "Open a codespace in VS Code",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				log.Errorln("<codespace> argument is deprecated. Use --codespace instead.")
				codespace = args[0]
			}
			return code(codespace, useInsiders)
		},
	}

	codeCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
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
