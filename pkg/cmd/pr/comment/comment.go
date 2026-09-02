package comment

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, telemetry ghtelemetry.CommandRecorder, runF func(*shared.CommentableOptions) error) *cobra.Command {
	opts := &shared.CommentableOptions{
		IO:                        f.IOStreams,
		HttpClient:                f.HttpClient,
		Config:                    f.Config,
		EditSurvey:                shared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey:     shared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:       shared.CommentableConfirmSubmitSurvey(f.Prompter),
		ConfirmCreateIfNoneSurvey: shared.CommentableInteractiveCreateIfNoneSurvey(f.Prompter),
		ConfirmDeleteLastComment:  shared.CommentableConfirmDeleteLastComment(f.Prompter),
		OpenInBrowser:             f.Browser.Browse,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "comment [<number> | <url> | <branch>]",
		Short: "Add a comment to a pull request",
		Long: heredoc.Docf(`
			Add a comment to a GitHub pull request.

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
			# Add a comment to a pull request
			$ gh pr comment 13 --body "Hi from GitHub CLI"

			# Attach a screenshot, with alt text after "#"
			$ gh pr comment 13 --attach './login.png#The login error state'

			# Attach multiple files by repeating the flag
			$ gh pr comment 13 --attach ./before.png --attach ./after.png
		`),
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.AttachFlag.RecordTelemetry(cmd.CommandPath(), telemetry)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}
			var selector string
			if len(args) > 0 {
				selector = args[0]
			}
			fields := []string{"id", "url"}
			if opts.EditLast || opts.DeleteLast {
				fields = append(fields, "comments")
			}
			finder := shared.NewFinder(f)
			opts.RetrieveCommentable = func() (shared.Commentable, ghrepo.Interface, error) {
				if len(opts.Assets) > 0 {
					fields = append(fields, "repository")
				}
				return finder.Find(shared.FindOptions{
					Selector: selector,
					Fields:   fields,
				})
			}
			if err := shared.CommentablePreRun(cmd, opts); err != nil {
				return err
			}

			// An asset is not a body, so an edit that only attaches keeps the
			// comment it is editing.
			opts.KeepExistingBody = true

			return nil
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
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "Edit the last comment of the current user")
	cmd.Flags().BoolVar(&opts.DeleteLast, "delete-last", false, "Delete the last comment of the current user")
	cmd.Flags().BoolVar(&opts.DeleteLastConfirmed, "yes", false, "Skip the delete confirmation prompt when --delete-last is provided")
	cmd.Flags().BoolVar(&opts.CreateIfNone, "create-if-none", false, "Create a new comment if no comments are found. Can be used only with --edit-last")
	opts.AttachFlag = attachments.AddFlag(cmd)

	return cmd
}
