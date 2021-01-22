package list

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	WebMode      bool
	LimitResults int
	State        string
	BaseBranch   string
	Labels       []string
	Assignee     string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List and filter pull requests in this repository",
		Example: heredoc.Doc(`
			$ gh pr list --limit 999
			$ gh pr list --state closed
			$ gh pr list --label "priority 1" --label "bug"
			$ gh pr list --web
    	`),
		Args: cmdutil.NoArgsQuoteReminder,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.LimitResults < 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("invalid value for --limit: %v", opts.LimitResults)}
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the browser to list the pull requests")
	cmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number of items to fetch")
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "Filter by state: {open|closed|merged|all}")
	cmd.Flags().StringVarP(&opts.BaseBranch, "base", "B", "", "Filter by base branch")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Filter by labels")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Filter by assignee")

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.WebMode {
		prListURL := ghrepo.GenerateRepoURL(baseRepo, "pulls")
		openURL, err := shared.ListURLWithQuery(prListURL, shared.FilterOptions{
			Entity:     "pr",
			State:      opts.State,
			Assignee:   opts.Assignee,
			Labels:     opts.Labels,
			BaseBranch: opts.BaseBranch,
		})
		if err != nil {
			return err
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	var graphqlState []string
	switch opts.State {
	case "open":
		graphqlState = []string{"OPEN"}
	case "closed":
		graphqlState = []string{"CLOSED", "MERGED"}
	case "merged":
		graphqlState = []string{"MERGED"}
	case "all":
		graphqlState = []string{"OPEN", "CLOSED", "MERGED"}
	default:
		return fmt.Errorf("invalid state: %s", opts.State)
	}

	params := map[string]interface{}{
		"state": graphqlState,
	}
	if len(opts.Labels) > 0 {
		params["labels"] = opts.Labels
	}
	if opts.BaseBranch != "" {
		params["baseBranch"] = opts.BaseBranch
	}
	if opts.Assignee != "" {
		params["assignee"] = opts.Assignee
	}

	listResult, err := api.PullRequestList(apiClient, baseRepo, params, opts.LimitResults)
	if err != nil {
		return err
	}

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if opts.IO.IsStdoutTTY() {
		hasFilters := opts.State != "open" || len(opts.Labels) > 0 || opts.BaseBranch != "" || opts.Assignee != ""
		title := shared.ListHeader(ghrepo.FullName(baseRepo), "pull request", len(listResult.PullRequests), listResult.TotalCount, hasFilters)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	cs := opts.IO.ColorScheme()
	table := utils.NewTablePrinter(opts.IO)
	for _, pr := range listResult.PullRequests {
		prNum := strconv.Itoa(pr.Number)
		if table.IsTTY() {
			prNum = "#" + prNum
		}
		table.AddField(prNum, nil, cs.ColorFromString(shared.ColorForPR(pr)))
		table.AddField(text.ReplaceExcessiveWhitespace(pr.Title), nil, nil)
		table.AddField(pr.HeadLabel(), nil, cs.Cyan)
		if !table.IsTTY() {
			table.AddField(prStateWithDraft(&pr), nil, nil)
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
