package prs

import (
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/search/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

func NewCmdPrs(f *cmdutil.Factory, runF func(*shared.IssuesOptions) error) *cobra.Command {
	var locked, merged bool
	var noAssignee, noLabel, noMilestone, noProject bool
	var order, sort string
	var appAuthor string
	var requestedReviewer string
	opts := &shared.IssuesOptions{
		Browser: f.Browser,
		Entity:  shared.PullRequests,
		IO:      f.IOStreams,
		Query: search.Query{Kind: search.KindIssues,
			Qualifiers: search.Qualifiers{Type: "pr"}},
	}

	cmd := &cobra.Command{
		Use:   "prs [<query>]",
		Short: "Search for pull requests",
		Long: heredoc.Doc(`
			Search for pull requests on GitHub.

			The command supports constructing queries using the GitHub search syntax,
			using the parameter and qualifier flags, or a combination of the two.

			GitHub search syntax is documented at:
			<https://docs.github.com/search-github/searching-on-github/searching-issues-and-pull-requests>
    `),
		Example: heredoc.Doc(`
			# search pull requests matching set of keywords "fix" and "bug"
			$ gh search prs fix bug

			# search draft pull requests in cli repository
			$ gh search prs --repo=cli/cli --draft

			# search open pull requests requesting your review
			$ gh search prs --review-requested=@me --state=open

			# search merged pull requests assigned to yourself
			$ gh search prs --assignee=@me --merged

			# search pull requests with numerous reactions
			$ gh search prs --reactions=">100"

			# search pull requests without label "bug"
			$ gh search prs -- -label:bug
    `),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 && c.Flags().NFlag() == 0 {
				return cmdutil.FlagErrorf("specify search keywords or flags")
			}
			if opts.Query.Limit < 1 || opts.Query.Limit > shared.SearchMaxResults {
				return cmdutil.FlagErrorf("`--limit` must be between 1 and 1000")
			}
			if c.Flags().Changed("author") && c.Flags().Changed("app") {
				return cmdutil.FlagErrorf("specify only `--author` or `--app`")
			}
			if c.Flags().Changed("app") {
				opts.Query.Qualifiers.Author = fmt.Sprintf("app/%s", appAuthor)
			}
			if c.Flags().Changed("order") {
				opts.Query.Order = order
			}
			if c.Flags().Changed("sort") {
				opts.Query.Sort = sort
			}
			if c.Flags().Changed("locked") && locked {
				if locked {
					opts.Query.Qualifiers.Is = append(opts.Query.Qualifiers.Is, "locked")
				} else {
					opts.Query.Qualifiers.Is = append(opts.Query.Qualifiers.Is, "unlocked")
				}
			}
			if c.Flags().Changed("merged") {
				if merged {
					opts.Query.Qualifiers.Is = append(opts.Query.Qualifiers.Is, "merged")
				} else {
					opts.Query.Qualifiers.Is = append(opts.Query.Qualifiers.Is, "unmerged")
				}
			}
			if c.Flags().Changed("no-assignee") && noAssignee {
				opts.Query.Qualifiers.No = append(opts.Query.Qualifiers.No, "assignee")
			}
			if c.Flags().Changed("no-label") && noLabel {
				opts.Query.Qualifiers.No = append(opts.Query.Qualifiers.No, "label")
			}
			if c.Flags().Changed("no-milestone") && noMilestone {
				opts.Query.Qualifiers.No = append(opts.Query.Qualifiers.No, "milestone")
			}
			if c.Flags().Changed("no-project") && noProject {
				opts.Query.Qualifiers.No = append(opts.Query.Qualifiers.No, "project")
			}
			if c.Flags().Changed("review-requested") {
				if strings.Contains(requestedReviewer, "/") {
					opts.Query.Qualifiers.TeamReviewRequested = requestedReviewer
				} else {
					opts.Query.Qualifiers.ReviewRequested = requestedReviewer
				}
			}
			opts.Query.Keywords = args
			if runF != nil {
				return runF(opts)
			}
			var err error
			opts.Searcher, err = shared.Searcher(f)
			if err != nil {
				return err
			}
			return shared.SearchIssues(opts)
		},
	}

	// Output flags
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, search.PullRequestFields)
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the search query in the web browser")

	// Query parameter flags
	cmd.Flags().IntVarP(&opts.Query.Limit, "limit", "L", 30, "Maximum number of results to fetch")
	cmdutil.StringEnumFlag(cmd, &order, "order", "", "desc", []string{"asc", "desc"}, "Order of results returned, ignored unless '--sort' flag is specified")
	cmdutil.StringEnumFlag(cmd, &sort, "sort", "", "best-match",
		[]string{
			"comments",
			"reactions",
			"reactions-+1",
			"reactions--1",
			"reactions-smile",
			"reactions-thinking_face",
			"reactions-heart",
			"reactions-tada",
			"interactions",
			"created",
			"updated",
		}, "Sort fetched results")

	// Issue query qualifier flags
	cmd.Flags().StringVar(&appAuthor, "app", "", "Filter by GitHub App author")
	cmdutil.NilBoolFlag(cmd, &opts.Query.Qualifiers.Archived, "archived", "", "Restrict search to archived repositories")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Assignee, "assignee", "", "Filter by assignee")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Author, "author", "", "Filter by author")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Closed, "closed", "", "Filter on closed at `date`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Commenter, "commenter", "", "Filter based on comments by `user`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Comments, "comments", "", "Filter on `number` of comments")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Created, "created", "", "Filter based on created at `date`")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Query.Qualifiers.In, "match", "", nil, []string{"title", "body", "comments"}, "Restrict search to specific field of issue")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Interactions, "interactions", "", "Filter on `number` of reactions and comments")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Involves, "involves", "", "Filter based on involvement of `user`")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Query.Qualifiers.Is, "visibility", "", nil, []string{"public", "private", "internal"}, "Filter based on repository visibility")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.Label, "label", nil, "Filter on label")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Language, "language", "", "Filter based on the coding language")
	cmd.Flags().BoolVar(&locked, "locked", false, "Filter on locked conversation status")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Mentions, "mentions", "", "Filter based on `user` mentions")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Milestone, "milestone", "", "Filter by milestone `title`")
	cmd.Flags().BoolVar(&noAssignee, "no-assignee", false, "Filter on missing assignee")
	cmd.Flags().BoolVar(&noLabel, "no-label", false, "Filter on missing label")
	cmd.Flags().BoolVar(&noMilestone, "no-milestone", false, "Filter on missing milestone")
	cmd.Flags().BoolVar(&noProject, "no-project", false, "Filter on missing project")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Project, "project", "", "Filter on project board `number`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Reactions, "reactions", "", "Filter on `number` of reactions")
	cmd.Flags().StringSliceVarP(&opts.Query.Qualifiers.Repo, "repo", "R", nil, "Filter on repository")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Qualifiers.State, "state", "", "", []string{"open", "closed"}, "Filter based on state")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Team, "team-mentions", "", "Filter based on team mentions")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Updated, "updated", "", "Filter on last updated at `date`")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.User, "owner", nil, "Filter on repository owner")

	// Pull request query qualifier flags
	cmd.Flags().StringVarP(&opts.Query.Qualifiers.Base, "base", "B", "", "Filter on base branch name")
	cmdutil.NilBoolFlag(cmd, &opts.Query.Qualifiers.Draft, "draft", "", "Filter based on draft state")
	cmd.Flags().StringVarP(&opts.Query.Qualifiers.Head, "head", "H", "", "Filter on head branch name")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Merged, "merged-at", "", "Filter on merged at `date`")
	cmd.Flags().BoolVar(&merged, "merged", false, "Filter based on merged state")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Qualifiers.Review, "review", "", "", []string{"none", "required", "approved", "changes_requested"}, "Filter based on review status")
	cmd.Flags().StringVar(&requestedReviewer, "review-requested", "", "Filter on `user` or team requested to review")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.ReviewedBy, "reviewed-by", "", "Filter on `user` who reviewed")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Qualifiers.Status, "checks", "", "", []string{"pending", "success", "failure"}, "Filter based on status of the checks")

	_ = cmdutil.RegisterBranchCompletionFlags(f.GitClient, cmd, "base", "head")

	return cmd
}
