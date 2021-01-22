package list

import (
	"fmt"
	"net/http"

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

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	WebMode bool

	Assignee     string
	Labels       []string
	State        string
	LimitResults int
	Author       string
	Mention      string
	Milestone    string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List and filter issues in this repository",
		Example: heredoc.Doc(`
			$ gh issue list -l "help wanted"
			$ gh issue list -A monalisa
			$ gh issue list -a @me
			$ gh issue list --web
			$ gh issue list --milestone 'MVP'
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
	cmd.Flags().IntVarP(&opts.LimitResults, "limit", "L", 30, "Maximum number of issues to fetch")
	cmd.Flags().StringVarP(&opts.Author, "author", "A", "", "Filter by author")
	cmd.Flags().StringVar(&opts.Mention, "mention", "", "Filter by mention")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "Filter by milestone `number` or `title`")

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

	isTerminal := opts.IO.IsStdoutTTY()

	meReplacer := shared.NewMeReplacer(apiClient, baseRepo.RepoHost())
	filterAssignee, err := meReplacer.Replace(opts.Assignee)
	if err != nil {
		return err
	}
	filterAuthor, err := meReplacer.Replace(opts.Author)
	if err != nil {
		return err
	}
	filterMention, err := meReplacer.Replace(opts.Mention)
	if err != nil {
		return err
	}

	if opts.WebMode {
		issueListURL := ghrepo.GenerateRepoURL(baseRepo, "issues")
		openURL, err := prShared.ListURLWithQuery(issueListURL, prShared.FilterOptions{
			Entity:    "issue",
			State:     opts.State,
			Assignee:  filterAssignee,
			Labels:    opts.Labels,
			Author:    filterAuthor,
			Mention:   filterMention,
			Milestone: opts.Milestone,
		})
		if err != nil {
			return err
		}
		if isTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	listResult, err := api.IssueList(apiClient, baseRepo, opts.State, opts.Labels, filterAssignee, opts.LimitResults, filterAuthor, filterMention, opts.Milestone)
	if err != nil {
		return err
	}

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if isTerminal {
		hasFilters := opts.State != "open" || len(opts.Labels) > 0 || opts.Assignee != "" || opts.Author != "" || opts.Mention != "" || opts.Milestone != ""
		title := prShared.ListHeader(ghrepo.FullName(baseRepo), "issue", len(listResult.Issues), listResult.TotalCount, hasFilters)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	issueShared.PrintIssues(opts.IO, "", len(listResult.Issues), listResult.Issues)

	return nil
}
