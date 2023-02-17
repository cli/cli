package comment

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, runF func(*prShared.CommentableOptions) error) *cobra.Command {
	opts := &prShared.CommentableOptions{
		IO:                    f.IOStreams,
		HttpClient:            f.HttpClient,
		EditSurvey:            prShared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey: prShared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:   prShared.CommentableConfirmSubmitSurvey(f.Prompter),
		OpenInBrowser:         f.Browser.Browse,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Add a comment to an issue",
		Long: heredoc.Doc(`
			Add a comment to a GitHub issue.

			Without the body text supplied through flags, the command will interactively
			prompt for the comment text.
		`),
		Example: heredoc.Doc(`
			$ gh issue comment 12 --body "Hi from GitHub CLI"
		`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RetrieveCommentable = func() (prShared.Commentable, ghrepo.Interface, error) {
				httpClient, err := f.HttpClient()
				if err != nil {
					return nil, nil, err
				}
				fields := []string{"id", "url"}
				if opts.EditLast {
					fields = append(fields, "comments")
				}
				return issueShared.IssueFromArgWithFields(httpClient, f.BaseRepo, args[0], fields)
			}
			return prShared.CommentablePreRun(cmd, opts)
		},
		RunE: func(_ *cobra.Command, args []string) error {
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
			return prShared.CommentableRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The comment body `text`")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	cmd.Flags().BoolP("editor", "e", false, "Skip prompts and open the text editor to write the body in")
	cmd.Flags().BoolP("web", "w", false, "Open the web browser to write the comment")
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "Edit the last comment of the same author")

	return cmd
}
