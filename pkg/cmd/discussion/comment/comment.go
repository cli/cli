package comment

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmd/discussion/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// CommentOptions holds the configuration for the discussion comment command.
type CommentOptions struct {
	IO       *iostreams.IOStreams
	BaseRepo func() (ghrepo.Interface, error)
	Client   func() (client.DiscussionClient, error)
	Prompter prompter.Prompter

	ParsedArg *shared.ParsedDiscussionOrCommentArg

	Body     string
	BodyFile string
	Edit     bool
	Delete   bool
	Yes      bool
}

// NewCmdComment returns the "discussion comment" command.
func NewCmdComment(f *cmdutil.Factory, runF func(*CommentOptions) error) *cobra.Command {
	opts := &CommentOptions{
		IO:       f.IOStreams,
		Prompter: f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "comment {<number> | <discussion-url> | <comment-id> | <comment-url>} [flags]",
		Short: "Add, edit, or delete a comment or a reply on a discussion (preview)",
		Long: heredoc.Docf(`
			Manage comments or replies on a GitHub discussion.

			The positional argument can be a discussion number or URL (to add a new
			top-level comment), or a comment node ID or comment URL (to reply, edit,
			or delete that comment).

			When the argument is a discussion number or URL, the default action is to
			add a new top-level comment. Likewise, if the argument is a comment URL or ID
			the default action is to add a reply.

			Use %[1]s--edit%[1]s to update the comment/reply body, or %[1]s--delete%[1]s to remove it.

			The body can be supplied via %[1]s--body%[1]s, %[1]s--body-file%[1]s, or interactively
			through an editor.
		`, "`"),
		Example: heredoc.Doc(`
			# Add a top-level comment to discussion #123
			$ gh discussion comment 123 --body 'Thanks'

			# Reply to a comment using its URL
			$ gh discussion comment 'https://github.com/OWNER/REPO/discussions/123#discussioncomment-456' --body 'Thanks'

			# Reply to a comment using its node ID
			$ gh discussion comment DC_abc123 --body 'Thanks'

			# Edit a comment/reply
			$ gh discussion comment 'https://github.com/OWNER/REPO/discussions/123#discussioncomment-456' --edit --body 'Thanks'

			# Delete a comment/reply
			$ gh discussion comment 'https://github.com/OWNER/REPO/discussions/123#discussioncomment-456' --delete

			# Delete a comment/reply without confirmation prompt
			$ gh discussion comment 'https://github.com/OWNER/REPO/discussions/123#discussioncomment-456' --delete --yes
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.Client = shared.DiscussionClientFunc(f)

			if err := cmdutil.MutuallyExclusive("specify only one of --edit or --delete",
				cmd.Flags().Changed("edit"), cmd.Flags().Changed("delete")); err != nil {
				return err
			}
			if opts.Delete {
				if cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file") {
					return cmdutil.FlagErrorf("--delete cannot be combined with --body or --body-file")
				}
			}
			if opts.Yes && !opts.Delete {
				return cmdutil.FlagErrorf("--yes can only be used with --delete")
			}
			if !opts.IO.CanPrompt() && opts.Delete && !opts.Yes {
				return cmdutil.FlagErrorf("--yes is required when not running interactively with --delete")
			}
			if !opts.IO.CanPrompt() && !opts.Delete {
				if opts.Body == "" && opts.BodyFile == "" {
					return cmdutil.FlagErrorf("--body or --body-file is required when not running interactively")
				}
			}
			if err := cmdutil.MutuallyExclusive("specify only one of --body or --body-file",
				cmd.Flags().Changed("body"), cmd.Flags().Changed("body-file")); err != nil {
				return err
			}

			parsed, err := shared.ParseDiscussionOrCommentArg(args[0])
			if err != nil {
				return err
			}

			opts.ParsedArg = parsed

			if (opts.Edit || opts.Delete) && (parsed.CommentNodeID == "" && parsed.CommentDatabaseID == 0) {
				return cmdutil.FlagErrorf("--edit and --delete require a comment ID or comment URL as argument")
			}

			if opts.ParsedArg.Repo != nil {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return parsed.Repo, nil
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return commentRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Comment body text")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	cmd.Flags().BoolVar(&opts.Edit, "edit", false, "Edit the specified comment")
	cmd.Flags().BoolVar(&opts.Delete, "delete", false, "Delete the specified comment")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "Skip the delete confirmation prompt")

	cmdutil.EnableRepoOverride(cmd, f)

	return cmd
}

func commentRun(opts *CommentOptions) error {
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}

	if opts.Delete {
		return runDelete(opts, c, baseRepo)
	}
	if opts.Edit {
		return runEdit(opts, c, baseRepo)
	}
	if opts.ParsedArg.CommentNodeID != "" || opts.ParsedArg.CommentDatabaseID != 0 {
		return runReply(opts, c, baseRepo)
	}
	return runAdd(opts, c, baseRepo)
}

func runDelete(opts *CommentOptions, c client.DiscussionClient, baseRepo ghrepo.Interface) error {
	commentID, err := resolveCommentID(opts, c, baseRepo)
	if err != nil {
		return err
	}

	if _, err := c.GetComment(baseRepo.RepoHost(), commentID); err != nil {
		return err
	}

	if !opts.Yes {
		confirmed, err := opts.Prompter.Confirm("Are you sure you want to delete this comment?", false)
		if err != nil {
			return err
		}
		if !confirmed {
			return cmdutil.CancelError
		}
	}

	return c.DeleteComment(baseRepo, commentID)
}

func runEdit(opts *CommentOptions, c client.DiscussionClient, baseRepo ghrepo.Interface) error {
	commentID, err := resolveCommentID(opts, c, baseRepo)
	if err != nil {
		return err
	}

	existing, err := c.GetComment(baseRepo.RepoHost(), commentID)
	if err != nil {
		return err
	}

	body, err := resolveBody(opts, existing.Body)
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	comment, err := c.UpdateComment(baseRepo, commentID, body)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, comment.URL)
	return nil
}

func runReply(opts *CommentOptions, c client.DiscussionClient, baseRepo ghrepo.Interface) error {
	commentID, err := resolveCommentID(opts, c, baseRepo)
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	existing, err := c.GetComment(baseRepo.RepoHost(), commentID)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	body, err := resolveBody(opts, "")
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	comment, err := c.AddComment(baseRepo, existing.DiscussionID, body, commentID)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, comment.URL)
	return nil
}

func runAdd(opts *CommentOptions, c client.DiscussionClient, baseRepo ghrepo.Interface) error {
	body, err := resolveBody(opts, "")
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	discussion, err := c.GetByNumber(baseRepo, opts.ParsedArg.Number)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	comment, err := c.AddComment(baseRepo, discussion.ID, body, "")
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, comment.URL)
	return nil
}

// resolveCommentID returns the comment node ID, resolving it from the database
// ID via the API if the arg was a comment URL.
func resolveCommentID(opts *CommentOptions, c client.DiscussionClient, repo ghrepo.Interface) (string, error) {
	if opts.ParsedArg.CommentNodeID != "" {
		return opts.ParsedArg.CommentNodeID, nil
	}
	if opts.ParsedArg.CommentDatabaseID != 0 {
		return c.ResolveCommentNodeID(repo, opts.ParsedArg.CommentDatabaseID)
	}
	// We should never reach here due to checks at flag parsing.
	return "", fmt.Errorf("no comment ID/URL available")
}

// resolveBody determines the comment body from flags or interactive input.
// defaultBody is used as the initial content in the editor (e.g., existing comment body for edits).
func resolveBody(opts *CommentOptions, defaultBody string) (string, error) {
	if opts.BodyFile != "" {
		b, err := cmdutil.ReadFile(opts.BodyFile, opts.IO.In)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	if opts.Body != "" {
		return opts.Body, nil
	}

	body, err := opts.Prompter.MarkdownEditor("Body", defaultBody, false)
	if err != nil {
		return "", err
	}

	return body, nil
}
