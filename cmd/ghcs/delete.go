package main

import (
	"context"
	"fmt"
	"os"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/spf13/cobra"
)

func NewDeleteCmd() *cobra.Command {
	deleteCmd := &cobra.Command{
		Use:   "delete [<codespace>]",
		Short: "Delete a Codespace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var codespaceName string
			if len(args) > 0 {
				codespaceName = args[0]
			}
			return Delete(codespaceName)
		},
	}

	deleteAllCmd := &cobra.Command{
		Use:   "all",
		Short: "Delete all Codespaces for the current user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return DeleteAll()
		},
	}

	deleteByRepoCmd := &cobra.Command{
		Use:   "repo <repo>",
		Short: "Delete all Codespaces for a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return DeleteByRepo(args[0])
		},
	}

	deleteCmd.AddCommand(deleteAllCmd, deleteByRepoCmd)

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

	codespace, token, err := codespaces.GetOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %v", err)
	}

	if err := apiClient.DeleteCodespace(ctx, user, token, codespace.Name); err != nil {
		return fmt.Errorf("error deleting codespace: %v", err)
	}

	fmt.Println("Codespace deleted.")

	return List()
}

func DeleteAll() error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %v", err)
	}

	for _, c := range codespaces {
		token, err := apiClient.GetCodespaceToken(ctx, user.Login, c.Name)
		if err != nil {
			return fmt.Errorf("error getting codespace token: %v", err)
		}

		if err := apiClient.DeleteCodespace(ctx, user, token, c.Name); err != nil {
			return fmt.Errorf("error deleting codespace: %v", err)
		}

		fmt.Printf("Codespace deleted: %s\n", c.Name)
	}

	return List()
}

func DeleteByRepo(repo string) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %v", err)
	}

	var deleted bool
	for _, c := range codespaces {
		if c.RepositoryNWO != repo {
			continue
		}
		deleted = true

		token, err := apiClient.GetCodespaceToken(ctx, user.Login, c.Name)
		if err != nil {
			return fmt.Errorf("error getting codespace token: %v", err)
		}

		if err := apiClient.DeleteCodespace(ctx, user, token, c.Name); err != nil {
			return fmt.Errorf("error deleting codespace: %v", err)
		}

		fmt.Printf("Codespace deleted: %s\n", c.Name)
	}

	if !deleted {
		fmt.Printf("No codespace was found for repository: %s\n", repo)
	}

	return List()
}
