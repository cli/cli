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
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	WebMode      bool
	LimitResults int
	Exporter     cmdutil.Exporter

	State      string
	BaseBranch string
	HeadBranch string
	Labels     []string
	Author     string
	Assignee   string
	Search     string
	Draft      *bool

	Now func() time.Time
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Now:        time.Now,
	}

	var appAuthor string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests in a repository",
		Long: heredoc.Doc(`
			List pull requests in a GitHub repository.

			The search query syntax is documented here:
			<https://docs.github.com/en/search-github/searching-on-github/searching-issues-and-pull-requests>
		`),
		Example: heredoc.Doc(`
			List PRs authored by you
			$ gh pr list --author "@me"

			List only PRs with all of the given labels
			$ gh pr list --label bug --label "priority 1"

			Filter PRs using search syntax
			$ gh pr list --search "status:success review:required"

			Find a PR that introduced a given commit
			$ gh pr list --search "<SHA>" --state merged
    	`),
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.LimitResults < 1 {
				return cmdutil.FlagErrorf("invalid value for --limit: %v", opts.LimitResults)
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

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List pull requests in the web browser")
	cmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number of items to fetch")
	cmdutil.StringEnumFlag(cmd, &opts.State, "state", "s", "open", []string{"open", "closed", "merged", "all"}, "Filter by state")
	cmd.Flags().StringVarP(&opts.BaseBranch, "base", "B", "", "Filter by base branch")
	cmd.Flags().StringVarP(&opts.HeadBranch, "head", "H", "", "Filter by head branch")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by label")
	cmd.Flags().StringVarP(&opts.Author, "author", "A", "", "Filter by author")
	cmd.Flags().StringVar(&appAuthor, "app", "", "Filter by GitHub App author")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Filter by assignee")
	cmd.Flags().StringVarP(&opts.Search, "search", "S", "", "Search pull requests with `query`")
	cmdutil.NilBoolFlag(cmd, &opts.Draft, "draft", "d", "Filter by draft state")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.PullRequestFields)

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "base", "head")

	return cmd
}

var defaultFields = []string{
	"number",
	"title",
	"state",
	"url",
	"headRefName",
	"headRepositoryOwner",
	"isCrossRepository",
	"isDraft",
	"createdAt",
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

	prState := strings.ToLower(opts.State)
	if prState == "open" && shared.QueryHasStateClause(opts.Search) {
		prState = ""
	}

	filters := shared.FilterOptions{
		Entity:     "pr",
		State:      prState,
		Author:     opts.Author,
		Assignee:   opts.Assignee,
		Labels:     opts.Labels,
		BaseBranch: opts.BaseBranch,
		HeadBranch: opts.HeadBranch,
		Search:     opts.Search,
		Draft:      opts.Draft,
		Fields:     defaultFields,
	}
	if opts.Exporter != nil {
		filters.Fields = opts.Exporter.Fields()
	}
	if opts.WebMode {
		prListURL := ghrepo.GenerateRepoURL(baseRepo, "pulls")
		openURL, err := shared.ListURLWithQuery(prListURL, filters)
		if err != nil {
			return err
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	listResult, err := listPullRequests(httpClient, baseRepo, filters, opts.LimitResults)
	if err != nil {
		return err
	}
	if len(listResult.PullRequests) == 0 && opts.Exporter == nil {
		return shared.ListNoResults(ghrepo.FullName(baseRepo), "pull request", !filters.IsDefault())
	}

	err = opts.IO.StartPager()
	if err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, listResult.PullRequests)
	}

	if listResult.SearchCapped {
		fmt.Fprintln(opts.IO.ErrOut, "warning: this query uses the Search API which is capped at 1000 results maximum")
	}
	if opts.IO.IsStdoutTTY() {
		title := shared.ListHeader(ghrepo.FullName(baseRepo), "pull request", len(listResult.PullRequests), listResult.TotalCount, !filters.IsDefault())
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	cs := opts.IO.ColorScheme()
	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	table := utils.NewTablePrinter(opts.IO)
	for _, pr := range listResult.PullRequests {
		prNum := strconv.Itoa(pr.Number)
		if table.IsTTY() {
			prNum = "#" + prNum
		}

		table.AddField(prNum, nil, cs.ColorFromString(shared.ColorForPRState(pr)))
		table.AddField(text.RemoveExcessiveWhitespace(pr.Title), nil, nil)
		table.AddField(pr.HeadLabel(), nil, cs.Cyan)
		if !table.IsTTY() {
			table.AddField(prStateWithDraft(&pr), nil, nil)
		}
		if table.IsTTY() {
			table.AddField(text.FuzzyAgo(opts.Now(), pr.CreatedAt), nil, cs.Gray)
		} else {
			table.AddField(pr.CreatedAt.String(), nil, nil)
		}
		table.EndRow()
	}
	err = table.Render()
	if err != nil {
		return err
	}

	return nil
}

func prStateWithDraft(pr *api.PullRequest) string {
	if pr.IsDraft && pr.State == "OPEN" {
		return "DRAFT"
	}

	return pr.State
}
