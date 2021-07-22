package main

import (
	"context"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"

	"github.com/github/ghcs/api"
	"github.com/spf13/cobra"
)

func NewListCmd() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List GitHub Codespaces you have on your account.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return List()
		},
	}

	return listCmd
}

func init() {
	rootCmd.AddCommand(NewListCmd())
}

func List() error {
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

	if len(codespaces) == 0 {
		fmt.Println("You have no codespaces.")
		return nil
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Repository", "Branch", "State", "Created At"})
	for _, codespace := range codespaces {
		table.Append([]string{
			codespace.Name, codespace.RepositoryNWO, codespace.Branch, codespace.Environment.State, codespace.CreatedAt,
		})
	}

	table.Render()
	return nil
}
