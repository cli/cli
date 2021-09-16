package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
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

	delete := func(name string) error {
		token, err := apiClient.GetCodespaceToken(ctx, user.Login, name)
		if err != nil {
			return fmt.Errorf("error getting codespace token: %v", err)
		}

		if err := apiClient.DeleteCodespace(ctx, user, token, name); err != nil {
			return fmt.Errorf("error deleting codespace: %v", err)
		}

		return nil
	}

	// Perform deletions in parallel.
	var (
		found bool
		mu    sync.Mutex // guards errs, logger
		errs  []error
		wg    sync.WaitGroup
	)
	for _, c := range codespaces {
		if !strings.EqualFold(c.RepositoryNWO, repo) {
			continue
		}
		found = true
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := delete(c.Name)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			} else {
				log.Printf("Codespace deleted: %s\n", c.Name)
			}
		}()
	}
	if !found {
		return fmt.Errorf("No codespace was found for repository: %s", repo)
	}
	wg.Wait()

	// Return first error, plus count of others.
	if errs != nil {
		err := errs[0]
		if others := len(errs) - 1; others > 0 {
			err = fmt.Errorf("%w (+%d more)", err, others)
		}
		return err
	}

	return list(&listOptions{})
}
