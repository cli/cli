package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/github/ghcs/api"
	"github.com/spf13/cobra"
)

func NewDeleteCmd() *cobra.Command {
	deleteCmd := &cobra.Command{
		Use:   "delete CODESPACE_NAME",
		Short: "Delete a GitHub Codespace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("A Codespace name is required.")
			}
			return Delete(args[0])
		},
	}

	return deleteCmd
}

func init() {
	rootCmd.AddCommand(NewDeleteCmd())
}

func Delete(codespaceName string) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	token, err := apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
	if err != nil {
		return fmt.Errorf("error getting codespace token: %v", err)
	}

	if err := apiClient.DeleteCodespace(ctx, user, token, codespaceName); err != nil {
		return fmt.Errorf("error deleting codespace: %v", err)
	}

	fmt.Println("Codespace deleted.")

	return List()
}
