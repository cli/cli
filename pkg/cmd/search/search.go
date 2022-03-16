package search

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"

	searchReposCmd "github.com/cli/cli/v2/pkg/cmd/search/repos"
)

func NewCmdSearch(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <command>",
		Short: "Search for repositories, issues, pull requests and users",
		Long:  "Search across all of GitHub.",
	}

	cmd.AddCommand(searchReposCmd.NewCmdRepos(f, nil))

	return cmd
}
