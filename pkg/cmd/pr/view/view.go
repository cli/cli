package view

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/briandowns/spinner"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	SelectorArg string
	BrowserMode bool
	Comments    bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "view [<number> | <url> | <branch>]",
		Short: "View a pull request",
		Long: heredoc.Doc(`
			Display the title, body, and other information about a pull request.

			Without an argument, the pull request that belongs to the current branch
			is displayed.

			With '--web', open the pull request in a web browser instead.
    	`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return &cmdutil.FlagError{Err: errors.New("argument required when using the --repo flag")}
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.BrowserMode, "web", "w", false, "Open a pull request in the browser")
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "View pull request comments")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, repo, err := shared.PRFromArgs(apiClient, opts.BaseRepo, opts.Branch, opts.Remotes, opts.SelectorArg)
	if err != nil {
		return err
	}

	connectedToTerminal := opts.IO.IsStdoutTTY() && opts.IO.IsStderrTTY()

	if opts.BrowserMode {
		openURL := pr.URL
		if connectedToTerminal {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	}

	if opts.Comments {
		var s *spinner.Spinner
		if connectedToTerminal {
			s = utils.Spinner(opts.IO.ErrOut)
			utils.StartSpinner(s)
		}

		comments, err := api.CommentsForPullRequest(apiClient, repo, pr)
		if err != nil {
			return err
		}
		pr.Comments = *comments

		if connectedToTerminal {
			utils.StopSpinner(s)
		}
	}

	opts.IO.DetectTerminalTheme()

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if connectedToTerminal {
		return printHumanPrPreview(opts.IO, pr)
	}

	if opts.Comments {
		return printRawPrComments(opts.IO.Out, pr)
	}

	return printRawPrPreview(opts.IO, pr)
}

func printRawPrPreview(io *iostreams.IOStreams, pr *api.PullRequest) error {
	out := io.Out
	cs := io.ColorScheme()

	reviewers := prReviewerList(*pr, cs)
	assignees := prAssigneeList(*pr)
	labels := prLabelList(*pr)
	projects := prProjectList(*pr)

	fmt.Fprintf(out, "title:\t%s\n", pr.Title)
	fmt.Fprintf(out, "state:\t%s\n", prStateWithDraft(pr))
	fmt.Fprintf(out, "author:\t%s\n", pr.Author.Login)
	fmt.Fprintf(out, "labels:\t%s\n", labels)
	fmt.Fprintf(out, "assignees:\t%s\n", assignees)
	fmt.Fprintf(out, "reviewers:\t%s\n", reviewers)
	fmt.Fprintf(out, "projects:\t%s\n", projects)
	fmt.Fprintf(out, "milestone:\t%s\n", pr.Milestone.Title)
	fmt.Fprintf(out, "number:\t%d\n", pr.Number)
	fmt.Fprintf(out, "url:\t%s\n", pr.URL)

	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, pr.Body)

	return nil
}

func printRawPrComments(out io.Writer, pr *api.PullRequest) error {
	var b strings.Builder
	for _, comment := range pr.Comments.Nodes {
		fmt.Fprint(&b, formatRawPrComment(comment))
	}
	fmt.Fprint(out, b.String())
	return nil
}

func formatRawPrComment(comment api.Comment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "author:\t%s\n", comment.Author.Login)
	fmt.Fprintf(&b, "association:\t%s\n", strings.ToLower(comment.AuthorAssociation))
	fmt.Fprintf(&b, "edited:\t%t\n", comment.IncludesCreatedEdit)
	fmt.Fprintln(&b, "--")
	fmt.Fprintln(&b, comment.Body)
	fmt.Fprintln(&b, "--")
	return b.String()
}

func printHumanPrPreview(io *iostreams.IOStreams, pr *api.PullRequest) error {
	out := io.Out
	cs := io.ColorScheme()

	// Header (Title and State)
	fmt.Fprintln(out, cs.Bold(pr.Title))
	fmt.Fprintf(out,
		"%s • %s wants to merge %s into %s from %s\n",
		shared.StateTitleWithColor(cs, *pr),
		pr.Author.Login,
		utils.Pluralize(pr.Commits.TotalCount, "commit"),
		pr.BaseRefName,
		pr.HeadRefName,
	)

	// Reactions
	if reactions := reactionGroupList(pr.ReactionGroups); reactions != "" {
		fmt.Fprint(out, reactions)
		fmt.Fprintln(out)
	}

	// Metadata
	if reviewers := prReviewerList(*pr, cs); reviewers != "" {
		fmt.Fprint(out, cs.Bold("Reviewers: "))
		fmt.Fprintln(out, reviewers)
	}
	if assignees := prAssigneeList(*pr); assignees != "" {
		fmt.Fprint(out, cs.Bold("Assignees: "))
		fmt.Fprintln(out, assignees)
	}
	if labels := prLabelList(*pr); labels != "" {
		fmt.Fprint(out, cs.Bold("Labels: "))
		fmt.Fprintln(out, labels)
	}
	if projects := prProjectList(*pr); projects != "" {
		fmt.Fprint(out, cs.Bold("Projects: "))
		fmt.Fprintln(out, projects)
	}
	if pr.Milestone.Title != "" {
		fmt.Fprint(out, cs.Bold("Milestone: "))
		fmt.Fprintln(out, pr.Milestone.Title)
	}

	// Body
	fmt.Fprintln(out)
	if pr.Body == "" {
		pr.Body = "_No description provided_"
	}
	style := markdown.GetStyle(io.TerminalTheme())
	md, err := markdown.Render(pr.Body, style, "")
	if err != nil {
		return err
	}
	fmt.Fprint(out, md)
	fmt.Fprintln(out)

	// Comments
	if pr.Comments.TotalCount > 0 {
		comments, err := prCommentList(io, pr.Comments)
		if err != nil {
			return err
		}
		fmt.Fprint(out, comments)
	}

	// Footer
	fmt.Fprintf(out, cs.Gray("View this pull request on GitHub: %s"), pr.URL)

	return nil
}

func prCommentList(io *iostreams.IOStreams, comments api.Comments) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()
	retrievedCount := len(comments.Nodes)
	hiddenCount := comments.TotalCount - retrievedCount

	if hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray(fmt.Sprintf("———————— Not showing %s ————————", utils.Pluralize(hiddenCount, "comment"))))
		fmt.Fprintf(&b, "\n\n\n")
	}

	for i, comment := range comments.Nodes {
		last := i+1 == retrievedCount
		cmt, err := formatPrComment(io, comment, last)
		if err != nil {
			return "", err
		}
		fmt.Fprint(&b, cmt)
		if last {
			fmt.Fprintln(&b)
		}
	}

	if hiddenCount > 0 {
		fmt.Fprint(&b, cs.Gray("Use --comments to view the full conversation"))
		fmt.Fprintln(&b)
	}

	return b.String(), nil
}

func formatPrComment(io *iostreams.IOStreams, comment api.Comment, newest bool) (string, error) {
	var b strings.Builder
	cs := io.ColorScheme()

	// Header
	fmt.Fprint(&b, cs.Bold(comment.Author.Login))
	if comment.AuthorAssociation != "NONE" {
		fmt.Fprint(&b, cs.Bold(fmt.Sprintf(" (%s)", strings.ToLower(comment.AuthorAssociation))))
	}
	fmt.Fprint(&b, cs.Bold(fmt.Sprintf(" • %s", utils.FuzzyAgoAbbr(time.Now(), comment.CreatedAt))))
	if comment.IncludesCreatedEdit {
		fmt.Fprint(&b, cs.Bold(" • edited"))
	}
	if newest {
		fmt.Fprint(&b, cs.Bold(" • "))
		fmt.Fprint(&b, cs.CyanBold("Newest comment"))
	}
	fmt.Fprintln(&b)

	// Reactions
	if reactions := reactionGroupList(comment.ReactionGroups); reactions != "" {
		fmt.Fprint(&b, reactions)
		fmt.Fprintln(&b)
	}

	// Body
	if comment.Body != "" {
		style := markdown.GetStyle(io.TerminalTheme())
		md, err := markdown.Render(comment.Body, style, "")
		if err != nil {
			return "", err
		}
		fmt.Fprint(&b, md)
	}

	return b.String(), nil
}

const (
	requestedReviewState        = "REQUESTED" // This is our own state for review request
	approvedReviewState         = "APPROVED"
	changesRequestedReviewState = "CHANGES_REQUESTED"
	commentedReviewState        = "COMMENTED"
	dismissedReviewState        = "DISMISSED"
	pendingReviewState          = "PENDING"
)

type reviewerState struct {
	Name  string
	State string
}

// formattedReviewerState formats a reviewerState with state color
func formattedReviewerState(cs *iostreams.ColorScheme, reviewer *reviewerState) string {
	state := reviewer.State
	if state == dismissedReviewState {
		// Show "DISMISSED" review as "COMMENTED", since "dismissed" only makes
		// sense when displayed in an events timeline but not in the final tally.
		state = commentedReviewState
	}

	var colorFunc func(string) string
	switch state {
	case requestedReviewState:
		colorFunc = cs.Yellow
	case approvedReviewState:
		colorFunc = cs.Green
	case changesRequestedReviewState:
		colorFunc = cs.Red
	default:
		colorFunc = func(str string) string { return str } // Do nothing
	}

	return fmt.Sprintf("%s (%s)", reviewer.Name, colorFunc(strings.ReplaceAll(strings.Title(strings.ToLower(state)), "_", " ")))
}

// prReviewerList generates a reviewer list with their last state
func prReviewerList(pr api.PullRequest, cs *iostreams.ColorScheme) string {
	reviewerStates := parseReviewers(pr)
	reviewers := make([]string, 0, len(reviewerStates))

	sortReviewerStates(reviewerStates)

	for _, reviewer := range reviewerStates {
		reviewers = append(reviewers, formattedReviewerState(cs, reviewer))
	}

	reviewerList := strings.Join(reviewers, ", ")

	return reviewerList
}

const teamTypeName = "Team"

const ghostName = "ghost"

// parseReviewers parses given Reviews and ReviewRequests
func parseReviewers(pr api.PullRequest) []*reviewerState {
	reviewerStates := make(map[string]*reviewerState)

	for _, review := range pr.Reviews.Nodes {
		if review.Author.Login != pr.Author.Login {
			name := review.Author.Login
			if name == "" {
				name = ghostName
			}
			reviewerStates[name] = &reviewerState{
				Name:  name,
				State: review.State,
			}
		}
	}

	// Overwrite reviewer's state if a review request for the same reviewer exists.
	for _, reviewRequest := range pr.ReviewRequests.Nodes {
		name := reviewRequest.RequestedReviewer.Login
		if reviewRequest.RequestedReviewer.TypeName == teamTypeName {
			name = reviewRequest.RequestedReviewer.Name
		}
		reviewerStates[name] = &reviewerState{
			Name:  name,
			State: requestedReviewState,
		}
	}

	// Convert map to slice for ease of sort
	result := make([]*reviewerState, 0, len(reviewerStates))
	for _, reviewer := range reviewerStates {
		if reviewer.State == pendingReviewState {
			continue
		}
		result = append(result, reviewer)
	}

	return result
}

// sortReviewerStates puts completed reviews before review requests and sorts names alphabetically
func sortReviewerStates(reviewerStates []*reviewerState) {
	sort.Slice(reviewerStates, func(i, j int) bool {
		if reviewerStates[i].State == requestedReviewState &&
			reviewerStates[j].State != requestedReviewState {
			return false
		}
		if reviewerStates[j].State == requestedReviewState &&
			reviewerStates[i].State != requestedReviewState {
			return true
		}

		return reviewerStates[i].Name < reviewerStates[j].Name
	})
}

func prAssigneeList(pr api.PullRequest) string {
	if len(pr.Assignees.Nodes) == 0 {
		return ""
	}

	AssigneeNames := make([]string, 0, len(pr.Assignees.Nodes))
	for _, assignee := range pr.Assignees.Nodes {
		AssigneeNames = append(AssigneeNames, assignee.Login)
	}

	list := strings.Join(AssigneeNames, ", ")
	if pr.Assignees.TotalCount > len(pr.Assignees.Nodes) {
		list += ", …"
	}
	return list
}

func prLabelList(pr api.PullRequest) string {
	if len(pr.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(pr.Labels.Nodes))
	for _, label := range pr.Labels.Nodes {
		labelNames = append(labelNames, label.Name)
	}

	list := strings.Join(labelNames, ", ")
	if pr.Labels.TotalCount > len(pr.Labels.Nodes) {
		list += ", …"
	}
	return list
}

func prProjectList(pr api.PullRequest) string {
	if len(pr.ProjectCards.Nodes) == 0 {
		return ""
	}

	projectNames := make([]string, 0, len(pr.ProjectCards.Nodes))
	for _, project := range pr.ProjectCards.Nodes {
		colName := project.Column.Name
		if colName == "" {
			colName = "Awaiting triage"
		}
		projectNames = append(projectNames, fmt.Sprintf("%s (%s)", project.Project.Name, colName))
	}

	list := strings.Join(projectNames, ", ")
	if pr.ProjectCards.TotalCount > len(pr.ProjectCards.Nodes) {
		list += ", …"
	}
	return list
}

func prStateWithDraft(pr *api.PullRequest) string {
	if pr.IsDraft && pr.State == "OPEN" {
		return "DRAFT"
	}

	return pr.State
}

func reactionGroupList(rgs api.ReactionGroups) string {
	var rs []string

	for _, rg := range rgs {
		if r := formatReactionGroup(rg); r != "" {
			rs = append(rs, r)
		}
	}

	return strings.Join(rs, " • ")
}

func formatReactionGroup(rg api.ReactionGroup) string {
	c := rg.Count()
	if c == 0 {
		return ""
	}
	e := rg.Emoji()
	if e == "" {
		return ""
	}
	return fmt.Sprintf("%v %s", c, e)
}
