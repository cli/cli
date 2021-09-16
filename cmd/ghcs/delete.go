package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/api"
	"github.com/spf13/cobra"
)

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
				return errors.New("both --all and --repo is not supported.")
			case allCodespaces && keepThresholdDays != 0:
				return deleteWithThreshold(log, keepThresholdDays)
			case allCodespaces:
				return deleteAll(log, force)
			case repo != "":
				return deleteByRepo(log, repo, force)
			default:
				return delete_(log, codespace, force)
			}
		},
	}

	deleteCmd.Flags().StringVarP(&codespace, "codespace", "c", "", "Name of the codespace")
	deleteCmd.Flags().BoolVar(&allCodespaces, "all", false, "Delete all codespaces")
	deleteCmd.Flags().StringVarP(&repo, "repo", "r", "", "Delete all codespaces for a repository")
	deleteCmd.Flags().BoolVarP(&force, "force", "f", false, "Delete codespaces with unsaved changes without confirmation")
	deleteCmd.Flags().IntVar(&keepThresholdDays, "days", 0, "Minimum number of days that a codespace has to have to be deleted. Only shutdown codespaces will be considered for deletion.")

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

func deleteAll(log *output.Logger, force bool) error {
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

	for _, c := range codespaces {
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

func deleteByRepo(log *output.Logger, repo string, force bool) error {
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

	var deleted bool
	for _, c := range codespaces {
		if !strings.EqualFold(c.RepositoryNWO, repo) {
			continue
		}

		confirmed, err := confirmDeletion(c, force)
		if err != nil {
			return fmt.Errorf("deletion could not be confirmed: %w", err)
		}

		if !confirmed {
			continue
		}

		deleted = true

		token, err := apiClient.GetCodespaceToken(ctx, user.Login, c.Name)
		if err != nil {
			return fmt.Errorf("error getting codespace token: %w", err)
		}

		if err := apiClient.DeleteCodespace(ctx, user, token, c.Name); err != nil {
			return fmt.Errorf("error deleting codespace: %w", err)
		}

		log.Printf("Codespace deleted: %s\n", c.Name)
	}

	if !deleted {
		return fmt.Errorf("No codespace was found for repository: %s", repo)
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

func deleteWithThreshold(log *output.Logger, keepThresholdDays int) error {
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

	codespacesToDelete, err := apiClient.FilterCodespacesToDelete(codespaces, keepThresholdDays)
	if err != nil {
		return err
	}

	for _, c := range codespacesToDelete {
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
