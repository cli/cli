package issue

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type ListOptions struct {
	SharedOptions

	State      string
	Assignee   string
	Limit      int
	Labels     []string
	Author     string
	HasFilters bool
}

var DefaultLimit = 30

func newListCmd(sharedOpts SharedOptions, testRunF func(*ListOptions)) *cobra.Command {
	var listCmd = &cobra.Command{
		Use:  "list",
		Args: cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := prepareListOpts(cmd, sharedOpts)
			if err != nil {
				return err
			}
			if testRunF != nil {
				testRunF(opts)
				return nil
			}
			return list(*opts)
		},
		Short: "List and filter issues in this repository",
		Example: heredoc.Doc(`
	$ gh issue list -l "help wanted"
	$ gh issue list -A monalisa
	`),
	}

	listCmd.Flags().StringP("assignee", "a", "", "Filter by assignee")
	listCmd.Flags().StringSliceP("label", "l", nil, "Filter by labels")
	listCmd.Flags().StringP("state", "s", "open", "Filter by state: {open|closed|all}")
	listCmd.Flags().IntP("limit", "L", DefaultLimit, "Maximum number of issues to fetch")
	listCmd.Flags().StringP("author", "A", "", "Filter by author")

	return listCmd
}

func prepareListOpts(cmd *cobra.Command, sharedOpts SharedOptions) (*ListOptions, error) {
	state, err := cmd.Flags().GetString("state")
	if err != nil {
		return nil, err
	}

	labels, err := cmd.Flags().GetStringSlice("label")
	if err != nil {
		return nil, err
	}

	assignee, err := cmd.Flags().GetString("assignee")
	if err != nil {
		return nil, err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, fmt.Errorf("invalid limit: %v", limit)
	}

	author, err := cmd.Flags().GetString("author")
	if err != nil {
		return nil, err
	}

	hasFilters := false
	cmd.Flags().Visit(func(f *pflag.Flag) {
		switch f.Name {
		case "state", "label", "assignee", "author":
			hasFilters = true
		}
	})

	opts := ListOptions{
		SharedOptions: sharedOpts,
		State:         state,
		Labels:        labels,
		Assignee:      assignee,
		Limit:         limit,
		Author:        author,
		HasFilters:    hasFilters,
	}
	return &opts, nil
}

func list(opts ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	client := api.NewClientFromHttpClient(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	listResult, err := api.IssueList(client, baseRepo, opts.State, opts.Labels, opts.Assignee, opts.Limit, opts.Author)
	if err != nil {
		return err
	}

	title := listHeader(ghrepo.FullName(baseRepo), "issue", len(listResult.Issues), listResult.TotalCount, opts.HasFilters)
	// TODO: avoid printing header if piped to a script
	fmt.Fprintf(opts.ColorableOut(), "\n%s\n\n", title)
	printIssues(opts.ColorableOut(), "", len(listResult.Issues), listResult.Issues)

	return nil
}
