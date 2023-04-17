package codespace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

type CodespaceSelector struct {
	api apiClient

	repoName      string
	codespaceName string
}

var errNoFilteredCodespaces = errors.New("you have no codespaces meeting the filter criteria")

// AddCodespaceSelector adds persistent flags for selecting a codespace to the given command and returns a CodespaceSelector which applies them
func AddCodespaceSelector(cmd *cobra.Command, api apiClient) *CodespaceSelector {
	cs := &CodespaceSelector{api: api}

	cmd.PersistentFlags().StringVarP(&cs.codespaceName, "codespace", "c", "", "Name of the codespace")
	cmd.PersistentFlags().StringVarP(&cs.repoName, "repo", "R", "", "Filter codespace selection by repository name (user/repo)")

	cmd.MarkFlagsMutuallyExclusive("codespace", "repo")

	return cs
}

func (cs *CodespaceSelector) Select(ctx context.Context) (codespace *api.Codespace, err error) {
	if cs.codespaceName != "" {
		codespace, err = cs.api.GetCodespace(ctx, cs.codespaceName, true)
		if err != nil {
			return nil, fmt.Errorf("getting full codespace details: %w", err)
		}
	} else {
		codespaces, err := cs.fetchCodespaces(ctx)
		if err != nil {
			return nil, err
		}

		codespace, err = cs.chooseCodespace(ctx, codespaces)
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

func (cs *CodespaceSelector) SelectName(ctx context.Context) (string, error) {
	if cs.codespaceName != "" {
		return cs.codespaceName, nil
	}

	codespaces, err := cs.fetchCodespaces(ctx)
	if err != nil {
		return "", err
	}

	codespace, err := cs.chooseCodespace(ctx, codespaces)
	if err != nil {
		return "", err
	}

	return codespace.Name, nil
}

func (cs *CodespaceSelector) fetchCodespaces(ctx context.Context) (codespaces []*api.Codespace, err error) {
	codespaces, err = cs.api.ListCodespaces(ctx, api.ListCodespacesOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting codespaces: %w", err)
	}

	if len(codespaces) == 0 {
		return nil, errNoCodespaces
	}

	// Note that repo filtering done here can also be done in api.ListCodespaces.
	// We do it here instead so that we can differentiate no codespaces in general vs. none after filtering.
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

	if len(codespaces) == 0 {
		return nil, errNoFilteredCodespaces
	}

	return codespaces, err
}

func (cs *CodespaceSelector) chooseCodespace(ctx context.Context, codespaces []*api.Codespace) (codespace *api.Codespace, err error) {
	skipPromptForSingleOption := cs.repoName != ""
	codespace, err = chooseCodespaceFromList(ctx, codespaces, false, skipPromptForSingleOption)
	if err != nil {
		if err == errNoCodespaces {
			return nil, err
		}
		return nil, fmt.Errorf("choosing codespace: %w", err)
	}

	return codespace, nil
}
