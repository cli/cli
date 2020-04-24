package command

import (
	"fmt"

	"github.com/cli/cli/api"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listRepoCmd)

	listRepoCmd.Flags().IntP("limit", "L", 30, "Maximum number of items to fetch")
	listRepoCmd.Flags().BoolP("raw", "", false, "Display raw output")
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List organizations or repositories",
}

var listRepoCmd = &cobra.Command{
	Use:   "repo",
	Short: "List repositories",
	RunE:  listRepos,
}

func listRepos(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)

	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	repos, err := api.Repositories(apiClient, limit)

	if err != nil {
		return err
	}

	raw, err := cmd.Flags().GetBool("raw")
	if err != nil {
		return err
	}

	if !raw {
		title := fmt.Sprintf("Showing %d of %d repositories.", len(repos.Nodes), repos.TotalCount)
		fmt.Fprintf(colorableErr(cmd), "\n%s\n\n", title)
	}

	table := utils.NewTablePrinter(cmd.OutOrStdout())
	for i := range repos.Nodes {
		table.AddField(repos.Nodes[i].NameWithOwner, nil, nil)
		table.EndRow()
	}
	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}
