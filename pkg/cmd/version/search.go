package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(searchCmd)
	searchCmd.Flags().IntP("limit", "L", 10, "limit the number of results")
}

var searchCmd = &cobra.Command{
	Use:   "search {repos|issues} <query>",
	Args:  cobra.ExactArgs(2),
	Short: "Search GitHub",
	Long: `Search GitHub repositories or issues using terms in a query string.

The literal string "{baseRepo}" inside the search string will be substituted with
the full name of the current repository. Similarly, "{owner}" will be replaced with
the handle of the owner for the current repository.

Examples:

	$ gh search repos 'in:readme "GitHub CLI"'
	$ gh search repos 'org:microsoft stars:>=15000'

	$ gh search issues 'repo:cli/cli editor'
	$ gh search issues 'repo:{baseRepo} is:open team:{owner}/robots'
	$ gh search issues 'repo:{baseRepo} is:pr is:open review-requested:@me'

See <https://help.github.com/en/github/searching-for-information-on-github>`,
	RunE: search,
}

func search(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return err
	}

	client, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	searchType := args[0]
	searchTerm := strings.ReplaceAll(args[1], "{baseRepo}", ghrepo.FullName(baseRepo))
	searchTerm = strings.ReplaceAll(searchTerm, "{owner}", baseRepo.RepoOwner())

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	switch searchType {
	case "repos":
		results, err := api.SearchRepos(client, searchTerm, limit)
		if err != nil {
			return err
		}

		table := utils.NewTablePrinter(cmd.OutOrStdout())
		for _, r := range results {
			table.AddField(r.NameWithOwner, nil, nil)
			table.AddField(fmt.Sprintf("%d", r.Stargazers.TotalCount), nil, utils.Yellow)
			table.AddField(replaceExcessiveWhitespace(r.Description), nil, nil)
			table.EndRow()
		}
		_ = table.Render()
	case "issues":
		results, err := api.SearchIssues(client, searchTerm, limit)
		if err != nil {
			return err
		}

		table := utils.NewTablePrinter(cmd.OutOrStdout())
		for _, i := range results {
			issueID := fmt.Sprintf("%s#%d", i.Repository.NameWithOwner, i.Number)
			createdAt := i.CreatedAt.Format(time.RFC3339)
			if table.IsTTY() {
				createdAt = utils.FuzzyAgo(time.Since(i.CreatedAt))
			}

			table.AddField(issueID, nil, colorFuncForState(i.State))
			table.AddField(replaceExcessiveWhitespace(i.Title), nil, nil)
			table.AddField(createdAt, nil, utils.Gray)
			table.EndRow()
		}
		_ = table.Render()
	default:
		return fmt.Errorf("unrecognized search type: %q", searchType)
	}

	return nil
}
