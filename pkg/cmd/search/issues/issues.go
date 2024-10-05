package issues

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/search/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

func NewCmdIssues(f *cmdutil.Factory, runF func(*shared.IssuesOptions) error) *cobra.Command {
	var locked, includePrs bool
	var noAssignee, noLabel, noMilestone, noProject bool
	var order, sort string
	var appAuthor string
	opts := &shared.IssuesOptions{
		Browser: f.Browser,
		Entity:  shared.Issues,
		IO:      f.IOStreams,
		Query: search.Query{Kind: search.KindIssues,
			Qualifiers: search.Qualifiers{Type: "issue"}},
	}

	cmd := &cobra.Command{
		Use:   "issues [<query>]",
		Short: "Search for issues",
		Long: heredoc.Doc(`
			Search for issues on GitHub.

			The command supports constructing queries using the GitHub search syntax,
			using the parameter and qualifier flags, or a combination of the two.

			GitHub search syntax is documented at:
			<https://docs.github.com/search-github/searching-on-github/searching-issues-and-pull-requests>
		`),
		Example: heredoc.Doc(`
			# search issues matching set of keywords "readme" and "typo"
			$ gh search issues readme typo

			# search issues matching phrase "broken feature"
			$ gh search issues "broken feature"

			# search issues and pull requests in cli organization
			$ gh search issues --include-prs --owner=cli

			# search open issues assigned to yourself
			$ gh search issues --assignee=@me --state=open

			# search issues with numerous comments
			$ gh search issues --comments=">100"

			# search issues without label "bug"
			$ gh search issues -- -label:bug

			# search issues only from un-archived repositories (default is all repositories)
			$ gh search issues --owner github --archived=false
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
			if includePrs {
				opts.Entity = shared.Both
				opts.Query.Qualifiers.Type = ""
			}
			if c.Flags().Changed("order") {
				opts.Query.Order = order
			}
			if c.Flags().Changed("sort") {
				opts.Query.Sort = sort
			}
			if c.Flags().Changed("locked") {
				if locked {
					opts.Query.Qualifiers.Is = append(opts.Query.Qualifiers.Is, "locked")
				} else {
					opts.Query.Qualifiers.Is = append(opts.Query.Qualifiers.Is, "unlocked")
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
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, search.IssueFields)
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the search query in the web browser")

	// Query parameter flags
	cmd.Flags().IntVarP(&opts.Query.Limit, "limit", "L", 30, "Maximum number of results to fetch")
	cmdutil.StringEnumFlag(cmd, &order, "order", "", "desc", []string{"asc", "desc"}, "Order of results returned, ignored unless '--sort' flag is specified")
	cmdutil.StringEnumFlag(cmd, &sort, "sort", "", "best-match",
		[]string{
			"comments",
			"created",
			"interactions",
			"reactions",
			"reactions-+1",
			"reactions--1",
			"reactions-heart",
			"reactions-smile",
			"reactions-tada",
			"reactions-thinking_face",
			"updated",
		}, "Sort fetched results")

	// Query qualifier flags
	cmd.Flags().BoolVar(&includePrs, "include-prs", false, "Include pull requests in results")
	cmd.Flags().StringVar(&appAuthor, "app", "", "Filter by GitHub App author")
	cmdutil.NilBoolFlag(cmd, &opts.Query.Qualifiers.Archived, "archived", "", "Filter based on the repository archived state {true|false}")
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
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Project, "project", "", "Filter on project board `owner/number`")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Reactions, "reactions", "", "Filter on `number` of reactions")
	cmd.Flags().StringSliceVarP(&opts.Query.Qualifiers.Repo, "repo", "R", nil, "Filter on repository")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Qualifiers.State, "state", "", "", []string{"open", "closed"}, "Filter based on state")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Team, "team-mentions", "", "Filter based on team mentions")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Updated, "updated", "", "Filter on last updated at `date`")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.User, "owner", nil, "Filter on repository owner")

	return cmd
}
