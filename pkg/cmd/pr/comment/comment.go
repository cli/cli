package comment

import (
	"errors"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, runF func(*shared.CommentableOptions) error) *cobra.Command {
	opts := &shared.CommentableOptions{
		IO:                    f.IOStreams,
		HttpClient:            f.HttpClient,
		EditSurvey:            shared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey: shared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:   shared.CommentableConfirmSubmitSurvey,
		OpenInBrowser:         f.Browser.Browse,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "comment [<number> | <url> | <branch>]",
		Short: "Create a new pr comment",
		Long: heredoc.Doc(`
			Create a new pr comment.

			Without an argument, the pull request that belongs to the current branch
			is selected.			

			With '--web', comment on the pull request in a web browser instead.
		`),
		Example: heredoc.Doc(`
			$ gh pr comment 22 --body "This looks great, lets get it deployed."
		`),
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return &cmdutil.FlagError{Err: errors.New("argument required when using the --repo flag")}
			}
			var selector string
			if len(args) > 0 {
				selector = args[0]
			}
			finder := shared.NewFinder(f)
			opts.RetrieveCommentable = func() (shared.Commentable, ghrepo.Interface, error) {
				return finder.Find(shared.FindOptions{
					Selector: selector,
					Fields:   []string{"id", "url"},
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

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file`")
	cmd.Flags().BoolP("editor", "e", false, "Add body using editor")
	cmd.Flags().BoolP("web", "w", false, "Add body in browser")

	return cmd
}
