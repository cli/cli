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

const (
	orderOldest = "oldest"
	orderNewest = "newest"
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

	DiscussionNumber  int32
	WebMode           bool
	Comments          bool
	CommentNodeID     string
	CommentDatabaseID int64
	Limit             int
	After             string
	Order             string
	Exporter          cmdutil.Exporter
	Now               func() time.Time
}

// NewCmdView creates the "discussion view" command.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:      f.IOStreams,
		Browser: f.Browser,
		Now:     time.Now,
	}

	cmd := &cobra.Command{
		Use:   "view {<number> | <discussion-url> | <comment-id> | <comment-url>} [flags]",
		Short: "View a discussion (preview)",
		Long: heredoc.Docf(`
			Display the title, body, and other information about a discussion.

			To see the comments on a discussion, pass %[1]s--comments%[1]s. A few latest replies
			of each comment will also be retrieved regardless of the selected ordering.

			To see the full reply thread of a single comment, pass a comment node ID or
			comment URL as the argument instead of a discussion
			(e.g., %[1]shttps://github.com/OWNER/REPO/discussions/123#discussioncomment-456%[1]s).

			Pagination and ordering can be controlled via %[1]s--order%[1]s, %[1]s--limit%[1]s, and %[1]s--after%[1]s flags.

			Use %[1]s--web%[1]s to open the discussion or comment in a web browser instead.
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

			# View the reply thread of a comment by node ID
			$ gh discussion view DC_abc123

			# View the reply thread of a comment by URL
			$ gh discussion view 'https://github.com/OWNER/REPO/discussions/123#discussioncomment-456'

			# Paginate through replies
			$ gh discussion view DC_abc123 --limit 10 --after CURSOR

			# Open in browser
			$ gh discussion view 123 --web
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of --comments or --web",
				opts.Comments, opts.WebMode); err != nil {
				return err
			}

			parsed, err := shared.ParseDiscussionOrCommentArg(args[0])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}

			if parsed.Repo != nil {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return parsed.Repo, nil
				}
			}

			opts.DiscussionNumber = parsed.Number
			opts.CommentNodeID = parsed.CommentNodeID
			opts.CommentDatabaseID = parsed.CommentDatabaseID

			repliesMode := opts.CommentNodeID != "" || opts.CommentDatabaseID != 0

			if repliesMode && opts.Comments {
				return cmdutil.FlagErrorf("--comments is not supported with a comment argument")
			}

			paginatedMode := repliesMode || needsComments(opts)
			if cmd.Flags().Changed("order") && !paginatedMode {
				return cmdutil.FlagErrorf("--order requires --comments or a comment argument")
			}
			if cmd.Flags().Changed("limit") && !paginatedMode {
				return cmdutil.FlagErrorf("--limit requires --comments or a comment argument")
			}
			if cmd.Flags().Changed("after") && !paginatedMode {
				return cmdutil.FlagErrorf("--after requires --comments or a comment argument")
			}
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %d", opts.Limit)
			}

			opts.Client = shared.DiscussionClientFunc(f)

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open a discussion in the browser")
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "View discussion comments")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of comments or replies to fetch")
	cmd.Flags().StringVar(&opts.After, "after", "", "Cursor for the next page")
	cmdutil.StringEnumFlag(cmd, &opts.Order, "order", "", orderNewest, []string{orderOldest, orderNewest}, "Order of comments or replies")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, discussionFields)

	return cmd
}

// resolveCommentNodeID returns the comment node ID for the current invocation,
// resolving it from a comment database ID (parsed from a comment URL) when the
// node ID is not already known.
func resolveCommentNodeID(c client.DiscussionClient, repo ghrepo.Interface, opts *ViewOptions) (string, error) {
	if opts.CommentNodeID != "" {
		return opts.CommentNodeID, nil
	}
	return c.ResolveCommentNodeID(repo, opts.CommentDatabaseID)
}

// needsComments returns true when the command should fetch full comment data,
// either because --comments was set or because --json requested the comments field.
func needsComments(opts *ViewOptions) bool {
	return opts.Comments || (opts.Exporter != nil && slices.Contains(opts.Exporter.Fields(), "comments"))
}

func viewRun(opts *ViewOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}

	repliesMode := opts.CommentNodeID != "" || opts.CommentDatabaseID != 0

	if opts.WebMode {
		if !repliesMode {
			openURL := ghrepo.GenerateRepoURL(repo, "discussions/%d", opts.DiscussionNumber)
			if opts.IO.IsStderrTTY() {
				fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
			}
			return opts.Browser.Browse(openURL)
		}

		opts.IO.StartProgressIndicator()
		commentID, err := resolveCommentNodeID(c, repo, opts)
		if err != nil {
			opts.IO.StopProgressIndicator()
			return err
		}
		comment, err := c.GetComment(repo.RepoHost(), commentID)
		opts.IO.StopProgressIndicator()
		if err != nil {
			return err
		}
		if opts.IO.IsStderrTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(comment.URL))
		}
		return opts.Browser.Browse(comment.URL)
	}

	opts.IO.DetectTerminalTheme()
	opts.IO.StartProgressIndicator()

	if repliesMode {
		commentID, err := resolveCommentNodeID(c, repo, opts)
		if err != nil {
			opts.IO.StopProgressIndicator()
			return err
		}

		discussion, err := c.GetCommentReplies(repo.RepoHost(), commentID, opts.Limit, opts.After, opts.Order == orderNewest)
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

		comment := discussion.Comments.Comments[0]
		if opts.IO.IsStdoutTTY() {
			return printHumanCommentAndReplies(opts, &comment)
		}
		return printRawReplies(opts.IO.Out, &comment)
	}

	var discussion *client.Discussion
	if needsComments(opts) {
		discussion, err = c.GetWithComments(repo, opts.DiscussionNumber, opts.Limit, opts.After, opts.Order == orderNewest)
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

	if opts.Comments {
		return printRawComments(opts.IO.Out, discussion.Comments)
	}

	return printRawView(opts.IO.Out, discussion)
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

	fmt.Fprintf(out, "%s • %s • %s %s • %s • %s\n",
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

		if d.Comments.Direction == client.DiscussionCommentListDirectionBackward {
			if shown := len(d.Comments.Comments); shown < d.Comments.TotalCount {
				remaining := d.Comments.TotalCount - shown
				pluralized := "comment"
				if remaining > 1 {
					pluralized = "comments"
				}
				fmt.Fprintf(out, "%s\n\n", cs.Muted(fmt.Sprintf("———————— Not showing older %d %s ————————", remaining, pluralized)))
			}
		}

		// The order of comments from the client is based on the order selected by the user (newest/oldest),
		// but we want to show them in chronological order to avoid confusion. So we need to reverse the slice
		// elements if it's a newest-first list.
		intuitivelyOrdered := slices.Clone(d.Comments.Comments)
		if d.Comments.Direction == client.DiscussionCommentListDirectionBackward {
			slices.Reverse(intuitivelyOrdered)
		}

		// Let's figure out if the last element in our list is actually the newest comment.
		// Note that we've already reordered the comments for display, so the "last" element
		// is always the newer in the list.
		lastIsNewest :=
			d.Comments.Cursor == "" && d.Comments.Direction == client.DiscussionCommentListDirectionBackward ||
				d.Comments.NextCursor == "" && d.Comments.Direction == client.DiscussionCommentListDirectionForward

		for i, c := range intuitivelyOrdered {
			isNewest := i == len(intuitivelyOrdered)-1 && lastIsNewest
			if err := printHumanComment(opts, out, c, "", false, isNewest); err != nil {
				return err
			}
		}

		if d.Comments.Direction == client.DiscussionCommentListDirectionForward {
			if shown := len(d.Comments.Comments); shown < d.Comments.TotalCount {
				remaining := d.Comments.TotalCount - shown
				pluralized := "comment"
				if remaining > 1 {
					pluralized = "comments"
				}
				fmt.Fprintf(out, "%s\n\n", cs.Muted(fmt.Sprintf("———————— Not showing newer %d %s ————————", remaining, pluralized)))
			}
		}

		if d.Comments.NextCursor != "" {
			fmt.Fprintf(out, cs.Muted("To see more comments, pass: --after %s\n"), d.Comments.NextCursor)
			fmt.Fprintln(out)
		}
	}

	fmt.Fprintf(out, cs.Muted("View this discussion on GitHub: %s\n"), d.URL)

	return nil
}

func printRawView(out io.Writer, d *client.Discussion) error {
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
	fmt.Fprintf(out, "number:\t%d\n", d.Number)
	fmt.Fprintf(out, "url:\t%s\n", d.URL)
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, d.Body)

	return nil
}

// printRawComments writes the comments as a sequence of metadata blocks,
// without any discussion-level fields or nested replies. Comments are
// printed in chronological order regardless of how they were fetched.
func printRawComments(out io.Writer, list client.DiscussionCommentList) error {
	comments := slices.Clone(list.Comments)
	if list.Direction == client.DiscussionCommentListDirectionBackward {
		slices.Reverse(comments)
	}

	for _, c := range comments {
		printRawComment(out, c)
	}

	return nil
}

func printHumanComment(opts *ViewOptions, out io.Writer, c client.DiscussionComment, indent string, isReply bool, isNewest bool) error {
	cs := opts.IO.ColorScheme()
	now := opts.Now()

	action := "commented"
	if isReply {
		action = "replied"
	}

	header := fmt.Sprintf("%s%s %s • %s",
		indent,
		cs.Bold(c.Author.Login),
		action,
		text.FuzzyAgoAbbr(now, c.CreatedAt),
	)
	if c.IsAnswer {
		header += fmt.Sprintf(" • %s %s", cs.SuccessIcon(), cs.Green("Answer"))
	}
	if isNewest {
		kind := "comment"
		if isReply {
			kind = "reply"
		}
		header += fmt.Sprintf(" • %s", fmt.Sprintf(cs.CyanBold("Newest %s"), kind))
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

	if isReply {
		// Replies are leaf nodes, so there won't be children replies/comments.
		return nil
	}

	if len(c.Replies.Comments) == 0 {
		return nil
	}

	if c.Replies.Direction == client.DiscussionCommentListDirectionBackward {
		if shown := len(c.Replies.Comments); shown < c.Replies.TotalCount {
			remaining := c.Replies.TotalCount - shown
			pluralized := "reply"
			if remaining > 1 {
				pluralized = "replies"
			}
			fmt.Fprintf(out, "%s%s\n\n", indent, cs.Muted(fmt.Sprintf("———————— Not showing older %d %s ————————", remaining, pluralized)))
		}
	}

	// The order of replies from the client is based on the order selected by the user (newest/oldest),
	// but we want to show them in chronological order to avoid confusion. So we need to reverse the slice
	// elements if it's a newest-first list.
	intuitivelyOrdered := slices.Clone(c.Replies.Comments)
	if c.Replies.Direction == client.DiscussionCommentListDirectionBackward {
		slices.Reverse(intuitivelyOrdered)
	}

	// Let's figure out if the last element in our list is actually the newest reply.
	// Note that we've already reordered the replies for display, so the "last" element
	// is always the newer in the list.
	lastIsNewest :=
		c.Replies.Cursor == "" && c.Replies.Direction == client.DiscussionCommentListDirectionBackward ||
			c.Replies.NextCursor == "" && c.Replies.Direction == client.DiscussionCommentListDirectionForward

	for i, reply := range intuitivelyOrdered {
		isNewest := i == len(intuitivelyOrdered)-1 && lastIsNewest
		if err := printHumanComment(opts, out, reply, indent+"  ", true, isNewest); err != nil {
			return err
		}
	}

	if c.Replies.Direction == client.DiscussionCommentListDirectionForward {
		if shown := len(c.Replies.Comments); shown < c.Replies.TotalCount {
			remaining := c.Replies.TotalCount - shown
			pluralized := "reply"
			if remaining > 1 {
				pluralized = "replies"
			}
			fmt.Fprintf(out, "%s%s\n\n", indent, cs.Muted(fmt.Sprintf("———————— Not showing newer %d %s ————————", remaining, pluralized)))
		}
	}

	return nil
}

func printRawComment(out io.Writer, c client.DiscussionComment) {
	fmt.Fprintf(out, "author:\t%s\n", c.Author.Login)
	fmt.Fprintf(out, "created:\t%s\n", c.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "url:\t%s\n", c.URL)
	if c.IsAnswer {
		fmt.Fprintln(out, "answer:\ttrue")
	}
	fmt.Fprintln(out, "--")
	fmt.Fprintln(out, c.Body)
	fmt.Fprintln(out, "--")
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

func printHumanCommentAndReplies(opts *ViewOptions, c *client.DiscussionComment) error {
	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	if err := printHumanComment(opts, out, *c, "", false, false); err != nil {
		return err
	}

	if c.Replies.NextCursor != "" {
		fmt.Fprintf(out, cs.Muted("To see more replies, pass: --after %s\n"), c.Replies.NextCursor)
		fmt.Fprintln(out)
	}

	return nil
}

// printRawReplies writes the replies of a comment as a sequence of metadata
// blocks, without any fields of the parent comment. Replies are printed in
// chronological order regardless of how they were fetched.
func printRawReplies(out io.Writer, c *client.DiscussionComment) error {
	replies := slices.Clone(c.Replies.Comments)
	if c.Replies.Direction == client.DiscussionCommentListDirectionBackward {
		slices.Reverse(replies)
	}

	for _, reply := range replies {
		printRawComment(out, reply)
	}

	return nil
}
