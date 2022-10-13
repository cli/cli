package list

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	Assignee     string
	Labels       []string
	State        string
	LimitResults int
	Author       string
	Mention      string
	Milestone    string
	Search       string
	WebMode      bool
	Exporter     cmdutil.Exporter

	Detector fd.Detector
	Now      func() time.Time
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
		Now:        time.Now,
	}

	var appAuthor string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues in a repository",
		Long: heredoc.Doc(`
			List issues in a GitHub repository.

			The search query syntax is documented here:
			<https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests>
		`),
		Example: heredoc.Doc(`
			$ gh issue list --label "bug" --label "help wanted"
			$ gh issue list --author monalisa
			$ gh issue list --assignee "@me"
			$ gh issue list --milestone "The big 1.0"
			$ gh issue list --search "error no:assignee sort:created-asc"
		`),
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.LimitResults < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.LimitResults)
			}

			if cmd.Flags().Changed("author") && cmd.Flags().Changed("app") {
				return cmdutil.FlagErrorf("specify only `--author` or `--app`")
			}

			if cmd.Flags().Changed("app") {
				opts.Author = fmt.Sprintf("app/%s", appAuthor)
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List issues in the web browser")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Filter by assignee")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by label")
	cmdutil.StringEnumFlag(cmd, &opts.State, "state", "s", "open", []string{"open", "closed", "all"}, "Filter by state")
	cmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number of issues to fetch")
	cmd.Flags().StringVarP(&opts.Author, "author", "A", "", "Filter by author")
	cmd.Flags().StringVar(&appAuthor, "app", "", "Filter by GitHub App author")
	cmd.Flags().StringVar(&opts.Mention, "mention", "", "Filter by mention")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Filter by milestone number or title")
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

	if opts.Detector == nil {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		opts.Detector = fd.NewDetector(cachedClient, baseRepo.RepoHost())
	}
	features, err := opts.Detector.IssueFeatures()
	if err != nil {
		return err
	}
	fields := defaultFields
	if features.StateReason {
		fields = append(defaultFields, "stateReason")
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
		Fields:    fields,
	}

	isTerminal := opts.IO.IsStdoutTTY()

	if opts.WebMode {
		issueListURL := ghrepo.GenerateRepoURL(baseRepo, "issues")
		openURL, err := prShared.ListURLWithQuery(issueListURL, filterOptions)
		if err != nil {
			return err
		}

		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
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
	if len(listResult.Issues) == 0 && opts.Exporter == nil {
		return prShared.ListNoResults(ghrepo.FullName(baseRepo), "issue", !filterOptions.IsDefault())
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, listResult.Issues)
	}

	if listResult.SearchCapped {
		fmt.Fprintln(opts.IO.ErrOut, "warning: this query uses the Search API which is capped at 1000 results maximum")
	}
	if isTerminal {
		title := prShared.ListHeader(ghrepo.FullName(baseRepo), "issue", len(listResult.Issues), listResult.TotalCount, !filterOptions.IsDefault())
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	issueShared.PrintIssues(opts.IO, opts.Now(), "", len(listResult.Issues), listResult.Issues)

	return nil
}

func issueList(client *http.Client, repo ghrepo.Interface, filters prShared.FilterOptions, limit int) (*api.IssuesAndTotalCount, error) {
	apiClient := api.NewClientFromHTTP(client)

	if filters.Search != "" || len(filters.Labels) > 0 || filters.Milestone != "" {
		if milestoneNumber, err := strconv.ParseInt(filters.Milestone, 10, 32); err == nil {
			milestone, err := milestoneByNumber(client, repo, int32(milestoneNumber))
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

func milestoneByNumber(client *http.Client, repo ghrepo.Interface, number int32) (*api.RepoMilestone, error) {
	var query struct {
		Repository struct {
			Milestone *api.RepoMilestone `graphql:"milestone(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(repo.RepoOwner()),
		"name":   githubv4.String(repo.RepoName()),
		"number": githubv4.Int(number),
	}

	gql := api.NewClientFromHTTP(client)
	if err := gql.Query(repo.RepoHost(), "RepositoryMilestoneByNumber", &query, variables); err != nil {
		return nil, err
	}
	if query.Repository.Milestone == nil {
		return nil, fmt.Errorf("no milestone found with number '%d'", number)
	}

	return query.Repository.Milestone, nil
}
