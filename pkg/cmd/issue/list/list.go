package list

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	issueShared "github.com/cli/cli/pkg/cmd/issue/shared"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser

	WebMode  bool
	Exporter cmdutil.Exporter

	Assignee     string
	Labels       []string
	State        string
	LimitResults int
	Author       string
	Mention      string
	Milestone    string
	Search       string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List and filter issues in this repository",
		Example: heredoc.Doc(`
			$ gh issue list -l "bug" -l "help wanted"
			$ gh issue list -A monalisa
			$ gh issue list -a "@me"
			$ gh issue list --web
			$ gh issue list --milestone "The big 1.0"
			$ gh issue list --search "error no:assignee sort:created-asc"
		`),
		Args: cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.LimitResults < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid limit: %v", opts.LimitResults)}
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the browser to list the issue(s)")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Filter by assignee")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by labels")
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: {open|closed|all}")
	_ = cmd.RegisterFlagCompletionFunc("state", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"open", "closed", "all"}, cobra.ShellCompDirectiveNoFileComp
	})
	cmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number of issues to fetch")
	cmd.Flags().StringVarP(&opts.Author, "author", "A", "", "Filter by author")
	cmd.Flags().StringVar(&opts.Mention, "mention", "", "Filter by mention")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Filter by milestone `number` or `title`")
	cmd.Flags().StringVarP(&opts.Search, "search", "S", "", "Search issues with `query`")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.IssueFields)

	return cmd
}

var defaultFields = []string{
	"number",
	"title",
	"url",
	"state",
	"updatedAt",
	"labels",
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	issueState := strings.ToLower(opts.State)
	if issueState == "open" && shared.QueryHasStateClause(opts.Search) {
		issueState = ""
	}

	filterOptions := prShared.FilterOptions{
		Entity:    "issue",
		State:     issueState,
		Assignee:  opts.Assignee,
		Labels:    opts.Labels,
		Author:    opts.Author,
		Mention:   opts.Mention,
		Milestone: opts.Milestone,
		Search:    opts.Search,
		Fields:    defaultFields,
	}

	isTerminal := opts.IO.IsStdoutTTY()

	if opts.WebMode {
		issueListURL := ghrepo.GenerateRepoURL(baseRepo, "issues")
		openURL, err := prShared.ListURLWithQuery(issueListURL, filterOptions)
		if err != nil {
			return err
		}

		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	if opts.Exporter != nil {
		filterOptions.Fields = opts.Exporter.Fields()
	}

	listResult, err := issueList(httpClient, baseRepo, filterOptions, opts.LimitResults)
	if err != nil {
		return err
	}

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, listResult.Issues)
	}

	if isTerminal {
		title := prShared.ListHeader(ghrepo.FullName(baseRepo), "issue", len(listResult.Issues), listResult.TotalCount, !filterOptions.IsDefault())
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	issueShared.PrintIssues(opts.IO, "", len(listResult.Issues), listResult.Issues)

	return nil
}

func issueList(client *http.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.IssuesAndTotalCount, error) {
	apiClient := api.NewClientFromHTTP(client)

	if filters.Search != "" || len(filters.Labels) > 0 {
		if milestoneNumber, err := strconv.ParseInt(filters.Milestone, 10, 32); err == nil {
			milestone, err := api.MilestoneByNumber(apiClient, repo, int32(milestoneNumber))
			if err != nil {
				return nil, err
			}
			filters.Milestone = milestone.Title
		}

		return searchIssues(apiClient, repo, filters, limit)
	}

	var err error
	meReplacer := shared.NewMeReplacer(apiClient, repo.RepoHost())
	filters.Assignee, err = meReplacer.Replace(filters.Assignee)
	if err != nil {
		return nil, err
	}
	filters.Author, err = meReplacer.Replace(filters.Author)
	if err != nil {
		return nil, err
	}
	filters.Mention, err = meReplacer.Replace(filters.Mention)
	if err != nil {
		return nil, err
	}

	return listIssues(apiClient, repo, filters, limit)
}
