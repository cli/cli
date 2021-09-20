package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/api"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type deleteOptions struct {
	deleteAll     bool
	skipConfirm   bool
	isInteractive bool
	codespaceName string
	repoFilter    string
	keepDays      uint16
	now           func() time.Time
	apiClient     *api.API
}

func newDeleteCmd() *cobra.Command {
	opts := deleteOptions{
		apiClient:     api.New(os.Getenv("GITHUB_TOKEN")),
		now:           time.Now,
		isInteractive: hasTTY,
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a codespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			// switch {
			// case allCodespaces && repo != "":
			// 	return errors.New("both --all and --repo is not supported")
			// case allCodespaces:
			// 	return deleteAll(log, force, keepThresholdDays)
			// case repo != "":
			// 	return deleteByRepo(log, repo, force, keepThresholdDays)
			log := output.NewLogger(os.Stdout, os.Stderr, false)
			return delete(context.Background(), log, opts)
		},
	}

	deleteCmd.Flags().StringVarP(&opts.codespaceName, "codespace", "c", "", "Delete codespace by `name`")
	deleteCmd.Flags().BoolVar(&opts.deleteAll, "all", false, "Delete all codespaces")
	deleteCmd.Flags().StringVarP(&opts.repoFilter, "repo", "r", "", "Delete codespaces for a repository")
	deleteCmd.Flags().BoolVarP(&opts.skipConfirm, "force", "f", false, "Skip confirmation for codespaces that contain unsaved changes")
	deleteCmd.Flags().Uint16Var(&opts.keepDays, "days", 0, "Delete codespaces older than `N` days")

	return deleteCmd
}

func init() {
	rootCmd.AddCommand(newDeleteCmd())
}

func delete(ctx context.Context, log *output.Logger, opts deleteOptions) error {
	user, err := opts.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespaces, err := opts.apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	nameFilter := opts.codespaceName
	if nameFilter == "" && !opts.deleteAll && opts.repoFilter == "" {
		c, err := chooseCodespaceFromList(ctx, codespaces)
		if err != nil {
			return fmt.Errorf("error choosing codespace: %w", err)
		}
		nameFilter = c.Name
	}

	var codespacesToDelete []*api.Codespace
	lastUpdatedCutoffTime := opts.now().AddDate(0, 0, -int(opts.keepDays))
	for _, c := range codespaces {
		if nameFilter != "" && c.Name != nameFilter {
			continue
		}
		if opts.repoFilter != "" && !strings.EqualFold(c.RepositoryNWO, opts.repoFilter) {
			continue
		}
		if opts.keepDays > 0 {
			t, err := time.Parse(time.RFC3339, c.LastUsedAt)
			if err != nil {
				return fmt.Errorf("error parsing last_used_at timestamp %q: %w", c.LastUsedAt, err)
			}
			if t.After(lastUpdatedCutoffTime) {
				continue
			}
		}
		if nameFilter == "" || !opts.skipConfirm {
			confirmed, err := confirmDeletion(c)
			if err != nil {
				return fmt.Errorf("deletion could not be confirmed: %w", err)
			}
			if !confirmed {
				continue
			}
		}
		codespacesToDelete = append(codespacesToDelete, c)
	}

	g := errgroup.Group{}
	for _, c := range codespacesToDelete {
		codespaceName := c.Name
		g.Go(func() error {
			token, err := opts.apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
			if err != nil {
				return fmt.Errorf("error getting codespace token: %w", err)
			}
			if err := opts.apiClient.DeleteCodespace(ctx, user, token, codespaceName); err != nil {
				return fmt.Errorf("error deleting codespace: %w", err)
			}
			return nil
		})
	}

	return g.Wait()
}

func confirmDeletion(codespace *api.Codespace) (bool, error) {
	gs := codespace.Environment.GitStatus
	hasUnsavedChanges := gs.HasUncommitedChanges || gs.HasUnpushedChanges
	if !hasUnsavedChanges {
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
