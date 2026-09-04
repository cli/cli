package comment

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, telemetry ghtelemetry.CommandRecorder, runF func(*prShared.CommentableOptions) error) *cobra.Command {
	opts := &prShared.CommentableOptions{
		IO:                        f.IOStreams,
		HttpClient:                f.HttpClient,
		Config:                    f.Config,
		EditSurvey:                prShared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey:     prShared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:       prShared.CommentableConfirmSubmitSurvey(f.Prompter),
		ConfirmCreateIfNoneSurvey: prShared.CommentableInteractiveCreateIfNoneSurvey(f.Prompter),
		ConfirmDeleteLastComment:  prShared.CommentableConfirmDeleteLastComment(f.Prompter),
		OpenInBrowser:             f.Browser.Browse,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Add a comment to an issue",
		Long: heredoc.Docf(`
			Add a comment to a GitHub issue.

			Without body text or attachments supplied through flags, the command will
			interactively prompt for the comment text.

			Use %[1]s--attach%[1]s to upload an image or video. If the body already references an
			attached file, such as %[1]s![alt](./login.png)%[1]s, that reference is rewritten to point
			at the uploaded asset. Any attached file the body does not reference is appended
			to the end of the comment.
			You can attach up to 50 files per command.

			Alt text for an image follows the path after %[1]s#%[1]s, as in
			%[1]s--attach './login.png#The login error state'%[1]s. Without it the filename is used.
			A reference already in the body keeps the alt text written there. Video renders
			as a player and has no alt text, so it cannot be given any.
		`, "`"),
		Example: heredoc.Doc(`
			# Add a comment to an issue
			$ gh issue comment 12 --body "Hi from GitHub CLI"

			# Attach a screenshot, with alt text after "#"
			$ gh issue comment 12 --attach './login.png#The login error state'

			# Attach multiple files by repeating the flag
			$ gh issue comment 12 --attach ./before.png --attach ./after.png
		`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RetrieveCommentable = func() (prShared.Commentable, ghrepo.Interface, error) {
				// TODO wm: more testing
				issueNumber, parsedBaseRepo, err := shared.ParseIssueFromArg(args[0])
				if err != nil {
					return nil, nil, err
				}

				// If the args provided the base repo then use that directly.
				var baseRepo ghrepo.Interface

				if parsedBaseRepo, present := parsedBaseRepo.Value(); present {
					baseRepo = parsedBaseRepo
				} else {
					// support `-R, --repo` override
					baseRepo, err = f.BaseRepo()
					if err != nil {
						return nil, nil, err
					}
				}

				httpClient, err := f.HttpClient()
				if err != nil {
					return nil, nil, err
				}

				fields := []string{"id", "url"}
				if opts.EditLast || opts.DeleteLast {
					fields = append(fields, "comments")
				}
				if len(opts.Assets) > 0 {
					fields = append(fields, "repository")
				}

				issue, err := issueShared.FindIssueOrPR(httpClient, baseRepo, issueNumber, fields)
				if err != nil {
					return nil, nil, err
				}

				return issue, baseRepo, nil
			}
			if err := prShared.CommentablePreRun(cmd, opts); err != nil {
				return err
			}

			// An asset is not a body, so an edit that only attaches keeps the
			// comment it is editing.
			opts.KeepExistingBody = true

			return nil
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
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "Edit the last comment of the current user")
	cmd.Flags().BoolVar(&opts.DeleteLast, "delete-last", false, "Delete the last comment of the current user")
	cmd.Flags().BoolVar(&opts.DeleteLastConfirmed, "yes", false, "Skip the delete confirmation prompt when --delete-last is provided")
	cmd.Flags().BoolVar(&opts.CreateIfNone, "create-if-none", false, "Create a new comment if no comments are found. Can be used only with --edit-last")
	opts.AttachFlag = attachments.AddFlag(cmd)
	opts.AttachTelemetry = attachments.NewInvocationTelemetry(opts.AttachFlag, telemetry)
	cmd.Args = opts.AttachTelemetry.WrapArgs(cmd.Args)

	return cmd
}
