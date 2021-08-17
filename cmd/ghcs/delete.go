package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/spf13/cobra"
)

func NewDeleteCmd() *cobra.Command {
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a GitHub Codespace.",
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
		Short: "delete all codespaces",
		Long:  "delete all codespaces for the user with the current token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return DeleteAll()
		},
	}

	deleteByRepoCmd := &cobra.Command{
		Use:   "repo REPO_NAME",
		Short: "delete all codespaces for the repo",
		Long: `delete all the codespaces that the user with the current token has in this repo.
This includes all codespaces in all states.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("A Repository name is required.")
			}
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
	log := output.NewLogger(os.Stdout, os.Stderr, false)

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

	log.Println("Codespace deleted.")

	return List(&ListOptions{})
}

func DeleteAll() error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()
	log := output.NewLogger(os.Stdout, os.Stderr, false)

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

		log.Printf("Codespace deleted: %s\n", c.Name)
	}

	return List(&ListOptions{})
}

func DeleteByRepo(repo string) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()
	log := output.NewLogger(os.Stdout, os.Stderr, false)

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

		log.Printf("Codespace deleted: %s\n", c.Name)
	}

	if !deleted {
		return fmt.Errorf("No codespace was found for repository: %s", repo)
	}

	return List(&ListOptions{})
}
