package command

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(searchCmd)
	searchCmd.AddCommand(repoSearch)

	repoSearch.Flags().StringP("sort", "s", "stars", "Sort by (default: stars)")
	repoSearch.Flags().Int16P("count", "c", 10, "Retrieve quantity (default: 10)")
}

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search for users or repositories by keyword",
	Long: `Search users or repositories.

A keyword can be supplied as an argument as any of the following formats:
- keyword
- "a long keyword string"
`,
}

var repoSearch = &cobra.Command{
	Use:   "repo",
	Args:  cobra.MinimumNArgs(1),
	Short: "Search a repository by keyword",
	RunE:  searchRepo,
}

func searchRepo(cmd *cobra.Command, args []string) error {
	// keyword := args[0]
	return nil
}
