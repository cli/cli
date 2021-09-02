package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	deleteCmd := &cobra.Command{
		Use:   "delete [<codespace>]",
		Short: "Delete a codespace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var codespaceName string
			if len(args) > 0 {
				codespaceName = args[0]
			}
			return delete_(codespaceName)
		},
	}

	deleteAllCmd := &cobra.Command{
		Use:   "all",
		Short: "Delete all codespaces for the current user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteAll()
		},
	}

	deleteByRepoCmd := &cobra.Command{
		Use:   "repo <repo>",
		Short: "Delete all codespaces for a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteByRepo(args[0])
		},
	}

	deleteCmd.AddCommand(deleteAllCmd, deleteByRepoCmd)

	return deleteCmd
}

func init() {
	rootCmd.AddCommand(newDeleteCmd())
}

func delete_(codespaceName string) error {
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

	return list(&listOptions{})
}

func deleteAll() error {
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

	return list(&listOptions{})
}

func deleteByRepo(repo string) error {
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
		if !strings.EqualFold(c.RepositoryNWO, repo) {
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

	return list(&listOptions{})
}
