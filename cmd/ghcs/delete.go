package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/api"
	"github.com/spf13/cobra"
)

var now func() time.Time = time.Now

func newDeleteCmd() *cobra.Command {
	var (
		codespace         string
		allCodespaces     bool
		repo              string
		force             bool
		keepThresholdDays int
	)

	log := output.NewLogger(os.Stdout, os.Stderr, false)
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a codespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case allCodespaces && repo != "":
				return errors.New("both --all and --repo is not supported")
			case allCodespaces:
				return deleteAll(log, force, keepThresholdDays)
			case repo != "":
				return deleteByRepo(log, repo, keepThresholdDays)
			default:
				return delete_(log, codespace, force)
			}
		},
	}

	deleteCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	deleteCmd.Flags().BoolVar(&allCodespaces, "all", false, "Delete all codespaces")
	deleteCmd.Flags().StringVarP(&repo, "repo", "r", "", "Delete all codespaces for a repository")
	deleteCmd.Flags().BoolVarP(&force, "force", "f", false, "Delete codespaces with unsaved changes without confirmation")
	deleteCmd.Flags().IntVar(&keepThresholdDays, "days", 0, "Minimum number of days since the codespace was created")

	return deleteCmd
}

func init() {
	rootCmd.AddCommand(newDeleteCmd())
}

func delete_(log *output.Logger, codespaceName string, force bool) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespace, token, err := getOrChooseCodespace(ctx, apiClient, user, codespaceName)
	if err != nil {
		return fmt.Errorf("get or choose codespace: %w", err)
	}

	confirmed, err := confirmDeletion(codespace, force)
	if err != nil {
		return fmt.Errorf("deletion could not be confirmed: %w", err)
	}

	if !confirmed {
		return nil
	}

	if err := apiClient.DeleteCodespace(ctx, user, token, codespace.Name); err != nil {
		return fmt.Errorf("error deleting codespace: %w", err)
	}

	log.Println("Codespace deleted.")

	return list(&listOptions{})
}

func deleteAll(log *output.Logger, force bool, keepThresholdDays int) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	codespacesToDelete, err := filterCodespacesToDelete(codespaces, keepThresholdDays)
	if err != nil {
		return err
	}

	for _, c := range codespacesToDelete {
		confirmed, err := confirmDeletion(c, force)
		if err != nil {
			return fmt.Errorf("deletion could not be confirmed: %w", err)
		}

		if !confirmed {
			continue
		}

		token, err := apiClient.GetCodespaceToken(ctx, user.Login, c.Name)
		if err != nil {
			return fmt.Errorf("error getting codespace token: %w", err)
		}

		if err := apiClient.DeleteCodespace(ctx, user, token, c.Name); err != nil {
			return fmt.Errorf("error deleting codespace: %w", err)
		}

		log.Printf("Codespace deleted: %s\n", c.Name)
	}

	return list(&listOptions{})
}

func deleteByRepo(log *output.Logger, repo string, keepThresholdDays int) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	codespaces, err = filterCodespacesToDelete(codespaces, keepThresholdDays)
	if err != nil {
		return err
	}

	delete := func(name string) error {
		token, err := apiClient.GetCodespaceToken(ctx, user.Login, name)
		if err != nil {
			return fmt.Errorf("error getting codespace token: %w", err)
		}

		if err := apiClient.DeleteCodespace(ctx, user, token, name); err != nil {
			return fmt.Errorf("error deleting codespace: %w", err)
		}

		return nil
	}

	// Perform deletions in parallel, for performance,
	// and to ensure all are attempted even if any one fails.
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
		return fmt.Errorf("no codespace was found for repository: %s", repo)
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

func confirmDeletion(codespace *api.Codespace, force bool) (bool, error) {
	gs := codespace.Environment.GitStatus
	hasUnsavedChanges := gs.HasUncommitedChanges || gs.HasUnpushedChanges
	if force || !hasUnsavedChanges {
		return true, nil
	}
	if !hasTTY {
		return false, fmt.Errorf("codespace %s has unsaved changes (use --force to override)", codespace.Name)
	}

	var confirmed struct {
		Confirmed bool
	}
	q := []*survey.Question{
		{
			Name: "confirmed",
			Prompt: &survey.Confirm{
				Message: fmt.Sprintf("Codespace %s has unsaved changes. OK to delete?", codespace.Name),
			},
		},
	}
	if err := ask(q, &confirmed); err != nil {
		return false, fmt.Errorf("failed to prompt: %w", err)
	}

	return confirmed.Confirmed, nil
}

func filterCodespacesToDelete(codespaces []*api.Codespace, keepThresholdDays int) ([]*api.Codespace, error) {
	if keepThresholdDays < 0 {
		return nil, fmt.Errorf("invalid value for threshold: %d", keepThresholdDays)
	}
	codespacesToDelete := []*api.Codespace{}
	for _, codespace := range codespaces {
		// get a date from a string representation
		t, err := time.Parse(time.RFC3339, codespace.LastUsedAt)
		if err != nil {
			return nil, fmt.Errorf("error parsing last used at date: %w", err)
		}
		if t.Before(now().AddDate(0, 0, -keepThresholdDays)) && codespace.Environment.State == "Shutdown" {
			codespacesToDelete = append(codespacesToDelete, codespace)
		}
	}
	return codespacesToDelete, nil
}
