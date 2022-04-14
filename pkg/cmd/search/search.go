package search

import (
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"

	searchIssuesCmd "github.com/cli/cli/v2/pkg/cmd/search/issues"
	searchPrsCmd "github.com/cli/cli/v2/pkg/cmd/search/prs"
	searchReposCmd "github.com/cli/cli/v2/pkg/cmd/search/repos"
)

func NewCmdSearch(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <command>",
		Short: "Search for repositories, issues, pull requests and users",
		Long:  "Search across all of GitHub.",
	}

	cmd.AddCommand(searchIssuesCmd.NewCmdIssues(f, nil))
	cmd.AddCommand(searchPrsCmd.NewCmdPrs(f, nil))
	cmd.AddCommand(searchReposCmd.NewCmdRepos(f, nil))

	return cmd
}
