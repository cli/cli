package comment

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, runF func(*shared.CommentableOptions) error) *cobra.Command {
	opts := &shared.CommentableOptions{
		IO:                    f.IOStreams,
		HttpClient:            f.HttpClient,
		EditSurvey:            shared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey: shared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:   shared.CommentableConfirmSubmitSurvey(f.Prompter),
		OpenInBrowser:         f.Browser.Browse,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "comment [<number> | <url> | <branch>]",
		Short: "Add a comment to a pull request",
		Long: heredoc.Doc(`
			Add a comment to a GitHub pull request.

			Without the body text supplied through flags, the command will interactively
			prompt for the comment text.
		`),
		Example: heredoc.Doc(`
			$ gh pr comment 13 --body "Hi from GitHub CLI"
		`),
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}
			var selector string
			if len(args) > 0 {
				selector = args[0]
			}
			fields := []string{"id", "url"}
			if opts.EditLast {
				fields = append(fields, "comments")
			}
			finder := shared.NewFinder(f)
			opts.RetrieveCommentable = func() (shared.Commentable, ghrepo.Interface, error) {
				return finder.Find(shared.FindOptions{
					Selector: selector,
					Fields:   fields,
				})
			}
			return shared.CommentablePreRun(cmd, opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if bodyFile != "" {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
			}

			if runF != nil {
				return runF(opts)
			}
			return shared.CommentableRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The comment body `text`")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().BoolP("editor", "e", false, "Skip prompts and open the text editor to write the body in")
	cmd.Flags().BoolP("web", "w", false, "Open the web browser to write the comment")
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "Edit the last comment of the same author")

	return cmd
}
