package codespaces

import (
	"context"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
)

func (a *App) List(ctx context.Context, asJSON bool, limit int) error {
	codespaces, err := a.apiClient.ListCodespaces(ctx, limit)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	table := output.NewTable(os.Stdout, asJSON)
	table.SetHeader([]string{"Name", "Repository", "Branch", "State", "Created At"})
	for _, apiCodespace := range codespaces {
		cs := codespace{apiCodespace}
		table.Append([]string{
			cs.Name,
			cs.Repository.FullName,
			cs.branchWithGitStatus(),
			cs.State,
			cs.CreatedAt,
		})
	}

	table.Render()
	return nil
}
