package codespace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

type CodespaceSelector interface {
	// Select fetches a codespace from the API according to the criteria set at its creation, prompting the user if needed
	Select(ctx context.Context) (codespace *api.Codespace, err error)

	// SelectName fetches a codespace name in the same way as Select, but may avoid API calls
	SelectName(ctx context.Context) (name string, err error)
}

type codespaceSelector struct {
	api apiClient

	repoName      string
	codespaceName string
}

var errNoFilteredCodespaces = errors.New("you have no codespaces meeting the filter criteria")

// AddCodespaceSelector add codespace filtering parameters to the give command and returns a CodespaceSelector which applies them
func AddCodespaceSelector(cmd *cobra.Command, api apiClient) *codespaceSelector {
	cs := &codespaceSelector{api: api}

	cmd.Flags().StringVarP(&cs.codespaceName, "codespace", "c", "", "Name of the codespace")
	cmd.Flags().StringVarP(&cs.repoName, "repo", "R", "", "Filter codespace selection by repository name (user/repo)")

	cmd.MarkFlagsMutuallyExclusive("codespace", "repo")

	return cs
}

// AddDeprecatedRepoShorthand adds a -r parameter (deprecated shorthand for --repo)
// which instructs the user to use -R instead.
func (cs *codespaceSelector) AddDeprecatedRepoShorthand(cmd *cobra.Command) error {
	cmd.Flags().StringVarP(&cs.repoName, "repo-deprecated", "r", "", "(Deprecated) Shorthand for --repo")

	if err := cmd.Flags().MarkHidden("repo-deprecated"); err != nil {
		return fmt.Errorf("error marking `-r` shorthand as hidden: %w", err)
	}

	if err := cmd.Flags().MarkShorthandDeprecated("repo-deprecated", "use `-R` instead"); err != nil {
		return fmt.Errorf("error marking `-r` shorthand as deprecated: %w", err)
	}

	cmd.MarkFlagsMutuallyExclusive("codespace", "repo-deprecated")

	return nil
}

func (cs *codespaceSelector) Select(ctx context.Context) (codespace *api.Codespace, err error) {
	if cs.codespaceName != "" {
		codespace, err = cs.api.GetCodespace(ctx, cs.codespaceName, true)
		if err != nil {
			return nil, fmt.Errorf("getting full codespace details: %w", err)
		}
	} else {
		codespace, err = cs.fetchAndChooseCodespace(ctx)
		if err != nil {
			return nil, err
		}
	}

	if codespace.PendingOperation {
		return nil, fmt.Errorf(
			"codespace is disabled while it has a pending operation: %s",
			codespace.PendingOperationDisabledReason,
		)
	}

	return codespace, nil
}

func (cs *codespaceSelector) SelectName(ctx context.Context) (string, error) {
	if cs.codespaceName != "" {
		return cs.codespaceName, nil
	}

	codespace, err := cs.fetchAndChooseCodespace(ctx)
	if err != nil {
		return "", err
	}

	return codespace.Name, nil
}

func (cs *codespaceSelector) fetchAndChooseCodespace(ctx context.Context) (codespace *api.Codespace, err error) {
	codespaces, err := cs.api.ListCodespaces(ctx, api.ListCodespacesOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting codespaces: %w", err)
	}

	if len(codespaces) == 0 {
		return nil, errNoCodespaces
	}

	codespaces = cs.filterCodespacesToChoose(codespaces)
	if len(codespaces) == 0 {
		return nil, errNoFilteredCodespaces
	}

	skipPromptForSingleOption := cs.repoName != ""
	codespace, err = chooseCodespaceFromList(ctx, codespaces, false, skipPromptForSingleOption)
	if err != nil {
		if err == errNoCodespaces {
			return nil, err
		}
		return nil, fmt.Errorf("choosing codespace: %w", err)
	}

	return codespace, err
}

func (cs *codespaceSelector) filterCodespacesToChoose(codespaces []*api.Codespace) []*api.Codespace {
	if cs.repoName != "" {
		var filteredCodespaces []*api.Codespace
		for _, c := range codespaces {
			if !strings.EqualFold(c.Repository.FullName, cs.repoName) {
				continue
			}

			filteredCodespaces = append(filteredCodespaces, c)
		}

		codespaces = filteredCodespaces
	}

	return codespaces
}
