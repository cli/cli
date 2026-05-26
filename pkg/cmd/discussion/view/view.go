package view

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmd/discussion/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

var discussionFields = []string{
	"id",
	"number",
	"title",
	"body",
	"url",
	"closed",
	"state",
	"stateReason",
	"author",
	"category",
	"labels",
	"answered",
	"answerChosenAt",
	"answerChosenBy",
	"comments",
	"reactionGroups",
	"createdAt",
	"updatedAt",
	"closedAt",
	"locked",
}

var reactionEmoji = map[string]string{
	"THUMBS_UP":   "\U0001f44d",
	"THUMBS_DOWN": "\U0001f44e",
	"LAUGH":       "\U0001f604",
	"HOORAY":      "\U0001f389",
	"CONFUSED":    "\U0001f615",
	"HEART":       "\u2764\ufe0f",
	"ROCKET":      "\U0001f680",
	"EYES":        "\U0001f440",
}

func reactionGroupList(groups []client.ReactionGroup) string {
	var parts []string
	for _, g := range groups {
		if g.TotalCount == 0 {
			continue
		}
		emoji := reactionEmoji[g.Content]
		if emoji == "" {
			emoji = g.Content
		}
		parts = append(parts, fmt.Sprintf("%s %d", emoji, g.TotalCount))
	}
	return strings.Join(parts, " • ")
}

// ViewOptions holds the configuration for the view command.
type ViewOptions struct {
	IO       *iostreams.IOStreams
	BaseRepo func() (ghrepo.Interface, error)
	Browser  browser.Browser
	Client   func() (client.DiscussionClient, error)

	DiscussionNumber int
	WebMode          bool
	Comments         bool
	Replies          string
	Limit            int
	After            string
	Order            string
	Exporter         cmdutil.Exporter
	Now              func() time.Time
}

// NewCmdView creates the "discussion view" command.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:      f.IOStreams,
		Browser: f.Browser,
		Now:     time.Now,
	}

	cmd := &cobra.Command{
		Use:   "view {<number> | <url>}",
		Short: "View a discussion (preview)",
		Long: heredoc.Docf(`
			Display the title, body, and other information about a discussion.

			With %[1]s--comments%[1]s flag, show threaded comments on the discussion.
			Use %[1]s--order%[1]s to control comment ordering (oldest or newest first).
			Use %[1]s--limit%[1]s and %[1]s--after%[1]s for paginating through comments.

			With %[1]s--replies%[1]s flag, show paginated replies on a specific comment.
			Pass the comment node ID (e.g. %[1]sDC_abc123%[1]s) to fetch its replies.
			Use %[1]s--limit%[1]s, %[1]s--after%[1]s, and %[1]s--order%[1]s to control reply pagination.

			With %[1]s--web%[1]s flag, open the discussion in a web browser instead.
		`, "`"),
		Example: heredoc.Doc(`
			# View a discussion by number
			$ gh discussion view 123

			# View a discussion by URL
			$ gh discussion view https://github.com/OWNER/REPO/discussions/123

			# View with comments
			$ gh discussion view 123 --comments

			# View with oldest comments first
			$ gh discussion view 123 --comments --order oldest

			# Limit to 10 comments
			$ gh discussion view 123 --comments --limit 10

			# Fetch the next page of comments
			$ gh discussion view 123 --comments --after CURSOR

			# View replies on a specific comment
			$ gh discussion view 123 --replies COMMENT-ID

			# Paginate through replies
			$ gh discussion view 123 --replies COMMENT-ID --limit 10 --after CURSOR

			# Open in browser
			$ gh discussion view 123 --web
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive("specify only one of --comments, --replies, or --web",
				opts.Comments, opts.Replies != "", opts.WebMode); err != nil {
				return err
			}

			repliesMode := opts.Replies != ""
			commentsMode := needsComments(opts)

			paginatedMode := commentsMode || repliesMode
			if cmd.Flags().Changed("order") && !paginatedMode {
				return cmdutil.FlagErrorf("--order requires --comments or --replies")
			}
			if cmd.Flags().Changed("limit") && !paginatedMode {
				return cmdutil.FlagErrorf("--limit requires --comments or --replies")
			}
			if cmd.Flags().Changed("after") && !paginatedMode {
				return cmdutil.FlagErrorf("--after requires --comments or --replies")
			}
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %d", opts.Limit)
			}

			number, repo, err := shared.ParseDiscussionArg(args[0])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}

			if repo != nil {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return repo, nil
				}
			} else {
				opts.BaseRepo = f.BaseRepo
			}

			opts.DiscussionNumber = number
			opts.Client = shared.DiscussionClientFunc(f)

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open a discussion in the browser")
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "View discussion comments")
	cmd.Flags().StringVar(&opts.Replies, "replies", "", "View replies on a specific comment by its node ID")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of comments or replies to fetch")
	cmd.Flags().StringVar(&opts.After, "after", "", "Cursor for the next page")
	cmdutil.StringEnumFlag(cmd, &opts.Order, "order", "", "newest", []string{"oldest", "newest"}, "Order of comments or replies")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, discussionFields)

	return cmd
}

// exporterNeedsComments returns true when the JSON exporter requests the comments field.
func exporterNeedsComments(exporter cmdutil.Exporter) bool {
	return slices.Contains(exporter.Fields(), "comments")
}

// needsComments returns true when the command should fetch full comment data,
// either because --comments was set or because --json requested the comments field.
func needsComments(opts *ViewOptions) bool {
	return opts.Comments || opts.Exporter != nil && exporterNeedsComments(opts.Exporter)
}

func viewRun(opts *ViewOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.WebMode {
		openURL := ghrepo.GenerateRepoURL(repo, "discussions/%d", opts.DiscussionNumber)
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}

	opts.IO.DetectTerminalTheme()
	opts.IO.StartProgressIndicator()

	if opts.Replies != "" {
		discussion, err := c.GetCommentReplies(repo, opts.DiscussionNumber, opts.Replies, opts.Limit, opts.After, opts.Order == "newest")
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}

		if opts.Exporter != nil {
			return opts.Exporter.Write(opts.IO, discussion)
		}

		if err := opts.IO.StartPager(); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
		}
		defer opts.IO.StopPager()

		if len(discussion.Comments.Comments) == 0 {
			return fmt.Errorf("no comment found for reply ID %s", opts.Replies)
		}
		comment := discussion.Comments.Comments[0]
		if opts.IO.IsStdoutTTY() {
			return printHumanReplies(opts, &comment)
		}
		return printRawReplies(opts.IO.Out, &comment)
	}

	var discussion *client.Discussion
	if needsComments(opts) {
		discussion, err = c.GetWithComments(repo, opts.DiscussionNumber, opts.Limit, opts.After, opts.Order == "newest")
	} else {
		discussion, err = c.GetByNumber(repo, opts.DiscussionNumber)
	}

	opts.IO.StopProgressIndicator()

	if err != nil {
		return err
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, discussion)
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.IO.IsStdoutTTY() {
		return printHumanView(opts, discussion)
	}

	return printRawView(opts.IO.Out, discussion, opts.Comments)
}

func printHumanView(opts *ViewOptions, d *client.Discussion) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	numberStr := fmt.Sprintf("#%d", d.Number)
	if !d.Closed {
		numberStr = cs.Green(numberStr)
	} else {
		numberStr = cs.Muted(numberStr)
	}
	fmt.Fprintf(out, "%s %s\n", cs.Bold(d.Title), numberStr)

	state := "Open"
	stateColor := cs.Green
	if d.Closed {
		state = "Closed"
		stateColor = cs.Muted
	}

	verb := "Started by"
	if d.Category.IsAnswerable {
		verb = "Asked by"
	}

	fmt.Fprintf(out, "%s · %s · %s %s · %s · %s\n",
		stateColor(state),
		d.Category.Name,
		verb,
		d.Author.Login,
		text.FuzzyAgo(opts.Now(), d.CreatedAt),
		text.Pluralize(d.Comments.TotalCount, "comment"),
	)

	if labels := labelList(d.Labels, cs); labels != "" {
		fmt.Fprint(out, cs.Bold("Labels: "))
		fmt.Fprintln(out, labels)
	}

	var md string
	if d.Body == "" {
		md = fmt.Sprintf("\n  %s\n\n", cs.Muted("No description provided"))
	} else {
		var err error
		md, err = markdown.Render(d.Body,
			markdown.WithTheme(opts.IO.TerminalTheme()),
			markdown.WithWrap(opts.IO.TerminalWidth()))
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "\n%s\n", md)

	if reactions := reactionGroupList(d.ReactionGroups); reactions != "" {
		fmt.Fprintln(out, reactions)
		fmt.Fprintln(out)
	}

	// Comments section
	if opts.Comments && d.Comments.TotalCount > 0 {
		fmt.Fprintln(out, cs.Bold("Comments"))
		fmt.Fprintln(out)

		for _, c := range d.Comments.Comments {
			if err := printHumanComment(opts, out, c, ""); err != nil {
				return err
			}
		}

		if shown := len(d.Comments.Comments); shown < d.Comments.TotalCount {
			remaining := d.Comments.TotalCount - shown
			age := "more"
			if d.Comments.Direction == client.DiscussionCommentListDirectionForward {
				age = "newer"
			} else if d.Comments.Direction == client.DiscussionCommentListDirectionBackward {
				age = "older"
			}
			fmt.Fprintf(out, cs.Muted("  And %d %s comments\n"), remaining, age)
			fmt.Fprintln(out)
		}

		if d.Comments.NextCursor != "" {
			fmt.Fprintf(out, cs.Muted("To see more comments, pass: --after %s\n"), d.Comments.NextCursor)
			fmt.Fprintln(out)
		}
	}

	fmt.Fprintf(out, cs.Muted("View this discussion on GitHub: %s\n"), d.URL)

	return nil
}

func printRawView(out io.Writer, d *client.Discussion, showComments bool) error {
	fmt.Fprintf(out, "title:\t%s\n", d.Title)
	state := "OPEN"
	if d.Closed {
		state = "CLOSED"
	}
	fmt.Fprintf(out, "state:\t%s\n", state)
	fmt.Fprintf(out, "category:\t%s\n", d.Category.Name)
	fmt.Fprintf(out, "author:\t%s\n", d.Author.Login)
	fmt.Fprintf(out, "labels:\t%s\n", labelList(d.Labels, nil))
	fmt.Fprintf(out, "comments:\t%d\n", d.Comments.TotalCount)
	if showComments && d.Comments.NextCursor != "" {
		fmt.Fprintf(out, "next:\t%s\n", d.Comments.NextCursor)
	}
	fmt.Fprintf(out, "number:\t%d\n", d.Number)
	fmt.Fprintf(out, "url:\t%s\n", d.URL)
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, d.Body)

	if showComments {
		for _, c := range d.Comments.Comments {
			printRawComment(out, c, "")
		}
	}

	return nil
}

func printHumanComment(opts *ViewOptions, out io.Writer, c client.DiscussionComment, indent string) error {
	cs := opts.IO.ColorScheme()
	now := opts.Now()

	header := fmt.Sprintf("%s%s commented %s",
		indent,
		cs.Bold(c.Author.Login),
		text.FuzzyAgo(now, c.CreatedAt),
	)
	if c.IsAnswer {
		header += " " + cs.Green("✓ Answer")
	}
	fmt.Fprintln(out, header)

	if c.Body != "" {
		md, err := markdown.Render(c.Body,
			markdown.WithTheme(opts.IO.TerminalTheme()),
			markdown.WithWrap(opts.IO.TerminalWidth()))
		if err != nil {
			return err
		}
		if indent != "" {
			md = text.Indent(md, indent)
		}
		fmt.Fprint(out, md)
	}

	if reactions := reactionGroupList(c.ReactionGroups); reactions != "" {
		fmt.Fprintf(out, "%s%s\n", indent, reactions)
	}

	fmt.Fprintln(out)

	for _, reply := range c.Replies.Comments {
		if err := printHumanComment(opts, out, reply, indent+"  "); err != nil {
			return err
		}
	}

	if shown := len(c.Replies.Comments); shown < c.Replies.TotalCount {
		directionLabel := "more"
		if c.Replies.Direction == client.DiscussionCommentListDirectionForward {
			directionLabel = "newer"
		} else if c.Replies.Direction == client.DiscussionCommentListDirectionBackward {
			directionLabel = "older"
		}
		fmt.Fprintf(out, "%s  %s\n\n", indent, cs.Muted(fmt.Sprintf("And %d %s replies", c.Replies.TotalCount-shown, directionLabel)))
	}

	return nil
}

func printRawComment(out io.Writer, c client.DiscussionComment, indent string) {
	answer := ""
	if c.IsAnswer {
		answer = "\tanswer"
	}
	fmt.Fprintf(out, "%scomment:\t%s\t%s\t%s%s\n", indent, c.Author.Login, c.CreatedAt.Format(time.RFC3339), c.URL, answer)
	fmt.Fprintf(out, "%s--\n", indent)
	if indent != "" {
		fmt.Fprint(out, text.Indent(c.Body, indent))
	} else {
		fmt.Fprint(out, c.Body)
	}
	fmt.Fprintln(out)

	for _, reply := range c.Replies.Comments {
		printRawComment(out, reply, indent+"  ")
	}
}

func labelList(labels []client.DiscussionLabel, cs *iostreams.ColorScheme) string {
	if len(labels) == 0 {
		return ""
	}

	sortedLabels := slices.Clone(labels)
	slices.SortStableFunc(sortedLabels, func(i, j client.DiscussionLabel) int {
		return strings.Compare(i.Name, j.Name)
	})

	names := make([]string, len(sortedLabels))
	for i, l := range sortedLabels {
		if cs == nil {
			names[i] = l.Name
		} else {
			names[i] = cs.Label(l.Color, l.Name)
		}
	}
	return strings.Join(names, ", ")
}

func printHumanReplies(opts *ViewOptions, c *client.DiscussionComment) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	if err := printHumanComment(opts, out, *c, ""); err != nil {
		return err
	}

	if c.Replies.NextCursor != "" {
		fmt.Fprintf(out, cs.Muted("To see more replies, pass: --after %s\n"), c.Replies.NextCursor)
		fmt.Fprintln(out)
	}

	return nil
}

func printRawReplies(out io.Writer, c *client.DiscussionComment) error {
	answer := ""
	if c.IsAnswer {
		answer = "\tanswer"
	}
	fmt.Fprintf(out, "comment:\t%s\t%s\t%s%s\n", c.Author.Login, c.CreatedAt.Format(time.RFC3339), c.URL, answer)
	fmt.Fprintf(out, "replies:\t%d\n", c.Replies.TotalCount)
	if c.Replies.NextCursor != "" {
		fmt.Fprintf(out, "next:\t%s\n", c.Replies.NextCursor)
	}
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, c.Body)

	for _, reply := range c.Replies.Comments {
		printRawComment(out, reply, "  ")
	}

	return nil
}
