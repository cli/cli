package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/api"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var (
		codespace     string
		allCodespaces bool
		repo          string
	)

	log := output.NewLogger(os.Stdout, os.Stderr, false)

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a codespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case allCodespaces && repo != "":
				return errors.New("both --all and --repo is not supported.")
			case allCodespaces:
				return deleteAll(log)
			case repo != "":
				return deleteByRepo(log, repo)
			default:
				return delete_(log, codespace)
			}
		},
	}

	deleteCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	deleteCmd.Flags().BoolVar(&allCodespaces, "all", false, "Delete all codespaces")
	deleteCmd.Flags().StringVarP(&repo, "repo", "r", "", "Delete all codespaces for a repository")

	return deleteCmd
}

func init() {
	rootCmd.AddCommand(newDeleteCmd())
}

func delete_(log *output.Logger, codespaceName string) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespace, token, err := getOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %v", err)
	}

	if err := apiClient.DeleteCodespace(ctx, user, token, codespace.Name); err != nil {
		return fmt.Errorf("error deleting codespace: %v", err)
	}

	log.Println("Codespace deleted.")

	return list(&listOptions{})
}

func deleteAll(log *output.Logger) error {
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

		log.Printf("Codespace deleted: %s\n", c.Name)
	}

	return list(&listOptions{})
}

func deleteByRepo(log *output.Logger, repo string) error {
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
