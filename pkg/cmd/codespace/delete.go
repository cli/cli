package ghcs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

type deleteOptions struct {
	deleteAll     bool
	skipConfirm   bool
	codespaceName string
	repoFilter    string
	keepDays      uint16

	isInteractive bool
	now           func() time.Time
	prompter      prompter
}

//go:generate moq -fmt goimports -rm -skip-ensure -out mock_prompter.go . prompter
type prompter interface {
	Confirm(message string) (bool, error)
}

func newDeleteCmd(app *App) *cobra.Command {
	opts := deleteOptions{
		isInteractive: hasTTY,
		now:           time.Now,
		prompter:      &surveyPrompter{},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.deleteAll && opts.repoFilter != "" {
				return errors.New("both --all and --repo is not supported")
			}
			return app.Delete(cmd.Context(), opts)
		},
	}

	deleteCmd.Flags().StringVarP(&opts.codespaceName, "codespace", "c", "", "Name of the codespace")
	deleteCmd.Flags().BoolVar(&opts.deleteAll, "all", false, "Delete all codespaces")
	deleteCmd.Flags().StringVarP(&opts.repoFilter, "repo", "r", "", "Delete codespaces for a `repository`")
	deleteCmd.Flags().BoolVarP(&opts.skipConfirm, "force", "f", false, "Skip confirmation for codespaces that contain unsaved changes")
	deleteCmd.Flags().Uint16Var(&opts.keepDays, "days", 0, "Delete codespaces older than `N` days")

	return deleteCmd
}

func (a *App) Delete(ctx context.Context, opts deleteOptions) error {
	user, err := a.apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	var codespaces []*api.Codespace
	nameFilter := opts.codespaceName
	if nameFilter == "" {
		codespaces, err = a.apiClient.ListCodespaces(ctx)
		if err != nil {
			return fmt.Errorf("error getting codespaces: %w", err)
		}

		if !opts.deleteAll && opts.repoFilter == "" {
			c, err := chooseCodespaceFromList(ctx, codespaces)
			if err != nil {
				return fmt.Errorf("error choosing codespace: %w", err)
			}
			nameFilter = c.Name
		}
	} else {
		// TODO: this token is discarded and then re-requested later in DeleteCodespace
		token, err := a.apiClient.GetCodespaceToken(ctx, user.Login, nameFilter)
		if err != nil {
			return fmt.Errorf("error getting codespace token: %w", err)
		}

		codespace, err := a.apiClient.GetCodespace(ctx, token, user.Login, nameFilter)
		if err != nil {
			return fmt.Errorf("error fetching codespace information: %w", err)
		}

		codespaces = []*api.Codespace{codespace}
	}

	codespacesToDelete := make([]*api.Codespace, 0, len(codespaces))
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
		if !opts.skipConfirm {
			confirmed, err := confirmDeletion(opts.prompter, c, opts.isInteractive)
			if err != nil {
				return fmt.Errorf("unable to confirm: %w", err)
			}
			if !confirmed {
				continue
			}
		}
		codespacesToDelete = append(codespacesToDelete, c)
	}

	if len(codespacesToDelete) == 0 {
		return errors.New("no codespaces to delete")
	}

	g := errgroup.Group{}
	for _, c := range codespacesToDelete {
		codespaceName := c.Name
		g.Go(func() error {
			if err := a.apiClient.DeleteCodespace(ctx, user.Login, codespaceName); err != nil {
				_, _ = a.logger.Errorf("error deleting codespace %q: %v\n", codespaceName, err)
				return err
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return errors.New("some codespaces failed to delete")
	}
	return nil
}

func confirmDeletion(p prompter, codespace *api.Codespace, isInteractive bool) (bool, error) {
	gs := codespace.Environment.GitStatus
	hasUnsavedChanges := gs.HasUncommitedChanges || gs.HasUnpushedChanges
	if !hasUnsavedChanges {
		return true, nil
	}
	if !isInteractive {
		return false, fmt.Errorf("codespace %s has unsaved changes (use --force to override)", codespace.Name)
	}
	return p.Confirm(fmt.Sprintf("Codespace %s has unsaved changes. OK to delete?", codespace.Name))
}

type surveyPrompter struct{}

func (p *surveyPrompter) Confirm(message string) (bool, error) {
	var confirmed struct {
		Confirmed bool
	}
	q := []*survey.Question{
		{
			Name: "confirmed",
			Prompt: &survey.Confirm{
				Message: message,
			},
		},
	}
	if err := ask(q, &confirmed); err != nil {
		return false, fmt.Errorf("failed to prompt: %w", err)
	}

	return confirmed.Confirmed, nil
}
