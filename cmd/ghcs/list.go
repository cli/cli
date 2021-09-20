package ghcs

import (
	"context"
	"fmt"
	"os"

	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/api"
	"github.com/spf13/cobra"
)

type listOptions struct {
	asJSON bool
}

func newListCmd() *cobra.Command {
	opts := &listOptions{}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List your codespaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return list(opts)
		},
	}

	listCmd.Flags().BoolVar(&opts.asJSON, "json", false, "Output as JSON")

	return listCmd
}

func list(opts *listOptions) error {
	apiClient := api.New(GithubToken)
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %w", err)
	}

	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %w", err)
	}

	table := output.NewTable(os.Stdout, opts.asJSON)
	table.SetHeader([]string{"Name", "Repository", "Branch", "State", "Created At"})
	for _, codespace := range codespaces {
		table.Append([]string{
			codespace.Name,
			codespace.RepositoryNWO,
			codespace.Branch + dirtyStar(codespace.Environment.GitStatus),
			codespace.Environment.State,
			codespace.CreatedAt,
		})
	}

	table.Render()
	return nil
}

func dirtyStar(status api.CodespaceEnvironmentGitStatus) string {
	if status.HasUncommitedChanges || status.HasUnpushedChanges {
		return "*"
	}

	return ""
}
