package codespaces

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"golang.org/x/sync/errgroup"
)

type DeleteOptions struct {
	DeleteAll     bool
	SkipConfirm   bool
	CodespaceName string
	RepoFilter    string
	KeepDays      uint16

	Now      func() time.Time
	Prompter prompter
}

//go:generate moq -fmt goimports -rm -skip-ensure -out mock_prompter.go . prompter
type prompter interface {
	Confirm(a *App, message string) (bool, error)
}

func (a *App) Delete(ctx context.Context, opts DeleteOptions) (err error) {
	var codespaces []*api.Codespace
	nameFilter := opts.CodespaceName

	if nameFilter == "" {
		codespaces, err = a.apiClient.ListCodespaces(ctx, -1)
		if err != nil {
			return fmt.Errorf("error getting codespaces: %w", err)
		}

		if !opts.DeleteAll && opts.RepoFilter == "" {
			c, err := a.chooseCodespaceFromList(ctx, codespaces)
			if err != nil {
				return fmt.Errorf("error choosing codespace: %w", err)
			}
			nameFilter = c.Name
		}
	} else {
		codespace, err := a.apiClient.GetCodespace(ctx, nameFilter, false)
		if err != nil {
			return fmt.Errorf("error fetching codespace information: %w", err)
		}

		codespaces = []*api.Codespace{codespace}
	}

	codespacesToDelete := make([]*api.Codespace, 0, len(codespaces))
	lastUpdatedCutoffTime := opts.Now().AddDate(0, 0, -int(opts.KeepDays))
	for _, c := range codespaces {
		if nameFilter != "" && c.Name != nameFilter {
			continue
		}
		if opts.RepoFilter != "" && !strings.EqualFold(c.Repository.FullName, opts.RepoFilter) {
			continue
		}
		if opts.KeepDays > 0 {
			t, err := time.Parse(time.RFC3339, c.LastUsedAt)
			if err != nil {
				return fmt.Errorf("error parsing last_used_at timestamp %q: %w", c.LastUsedAt, err)
			}
			if t.After(lastUpdatedCutoffTime) {
				continue
			}
		}
		if !opts.SkipConfirm {
			confirmed, err := confirmDeletion(a, opts.Prompter, c, a.isInteractive)
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
			if err := a.apiClient.DeleteCodespace(ctx, codespaceName); err != nil {
				a.errLogger.Printf("error deleting codespace %q: %v\n", codespaceName, err)
				return err
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return errors.New("some codespaces failed to delete")
	}

	noun := "Codespace"
	if len(codespacesToDelete) > 1 {
		noun = noun + "s"
	}
	a.logger.Println(noun + " deleted.")

	return nil
}

type surveyPrompter struct{}

func (p *surveyPrompter) Confirm(a *App, message string) (bool, error) {
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
	if err := a.ask(q, &confirmed); err != nil {
		return false, fmt.Errorf("failed to prompt: %w", err)
	}

	return confirmed.Confirmed, nil
}

func confirmDeletion(a *App, p prompter, apiCodespace *api.Codespace, isInteractive bool) (bool, error) {
	cs := codespace{apiCodespace}
	if !cs.hasUnsavedChanges() {
		return true, nil
	}
	if !isInteractive {
		return false, fmt.Errorf("codespace %s has unsaved changes (use --force to override)", cs.Name)
	}
	return p.Confirm(a, fmt.Sprintf("Codespace %s has unsaved changes. OK to delete?", cs.Name))
}
