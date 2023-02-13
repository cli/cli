package codespace

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

type CodespaceSelector interface {
	Select(ctx context.Context) (codespace *api.Codespace, err error)
}

type codespaceSelector struct {
	api apiClient

	repoName      string
	codespaceName string
}

var errNoFilteredCodespaces = errors.New("you have no codespaces meeting the filter criteria")

func AddCodespaceSelector(cmd *cobra.Command, app *App) CodespaceSelector {
	cs := &codespaceSelector{api: app.apiClient}

	cmd.Flags().StringVarP(&cs.codespaceName, "codespace", "c", "", "Name of the codespace")
	cmd.Flags().StringVarP(&cs.repoName, "repo", "R", "", "Filter codespace selection by repository name (user/repo)")

	cmd.MarkFlagsMutuallyExclusive("codespace", "repo")

	return cs
}

func (c *codespaceSelector) Select(ctx context.Context) (codespace *api.Codespace, err error) {
	if c.codespaceName != "" {
		codespace, err = c.api.GetCodespace(ctx, c.codespaceName, true)
		if err != nil {
			return nil, fmt.Errorf("getting full codespace details: %w", err)
		}
	} else {
		codespace, err = c.chooseCodespace(ctx)
		if err != nil {
			if err == errNoCodespaces {
				return nil, err
			}
			return nil, fmt.Errorf("choosing codespace: %w", err)
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

func (c *codespaceSelector) chooseCodespace(ctx context.Context) (*api.Codespace, error) {
	codespaces, err := c.api.ListCodespaces(ctx, api.ListCodespacesOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting codespaces: %w", err)
	}

	codespaces = filterCodespacesToChoose(codespaces, c.repoName)
	if len(codespaces) == 0 {
		return nil, errNoFilteredCodespaces
	}

	skipPromptForSingleOption := c.repoName != ""
	return chooseCodespaceFromList(ctx, codespaces, false, skipPromptForSingleOption)
}

func filterCodespacesToChoose(codespaces []*api.Codespace, repoName string) []*api.Codespace {
	if repoName != "" {
		var filteredCodespaces []*api.Codespace
		for _, c := range codespaces {
			if c.Repository.FullName != repoName {
				continue
			}

			filteredCodespaces = append(filteredCodespaces, c)
		}

		codespaces = filteredCodespaces
	}

	return codespaces
}
