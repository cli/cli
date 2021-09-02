package main

import (
	"context"
	"fmt"
	"os"

	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
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

func init() {
	rootCmd.AddCommand(newListCmd())
}

func list(opts *listOptions) error {
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	ctx := context.Background()

	user, err := apiClient.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting user: %v", err)
	}

	codespaces, err := apiClient.ListCodespaces(ctx, user)
	if err != nil {
		return fmt.Errorf("error getting codespaces: %v", err)
	}

	table := output.NewTable(os.Stdout, opts.asJSON)
	table.SetHeader([]string{"Name", "Repository", "Branch", "State", "Created At"})
	for _, codespace := range codespaces {
		table.Append([]string{
			codespace.Name, codespace.RepositoryNWO, codespace.Branch, codespace.Environment.State, codespace.CreatedAt,
		})
	}

	table.Render()
	return nil
}
