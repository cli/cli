package shared

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

var errNoUserComments = errors.New("no comments found for current user")
var errDeleteNotConfirmed = errors.New("deletion not confirmed")

type InputType int

const (
	InputTypeEditor InputType = iota
	InputTypeInline
	InputTypeWeb
)

type Commentable interface {
	Link() string
	Identifier() string
	CurrentUserComments() []api.Comment
	RepositoryDatabaseID() int64
	RepositoryViewerPermission() string
}

type CommentableOptions struct {
	IO                        *iostreams.IOStreams
	HttpClient                func() (*http.Client, error)
	RetrieveCommentable       func() (Commentable, ghrepo.Interface, error)
	EditSurvey                func(string) (string, error)
	InteractiveEditSurvey     func(string) (string, error)
	ConfirmSubmitSurvey       func() (bool, error)
	ConfirmCreateIfNoneSurvey func() (bool, error)
	ConfirmDeleteLastComment  func(string) (bool, error)
	OpenInBrowser             func(string) error
	Interactive               bool
	InputType                 InputType
	Body                      string
	EditLast                  bool
	DeleteLast                bool
	DeleteLastConfirmed       bool
	CreateIfNone              bool
	Quiet                     bool
	Host                      string

	BodyProvided     bool
	KeepExistingBody bool
	AttachFlag       *attachments.Flag
	Assets           []attachments.UserAsset
	Config           func() (gh.Config, error)
}

func CommentablePreRun(cmd *cobra.Command, opts *CommentableOptions) error {
	inputFlags := 0
	if cmd.Flags().Changed("body") {
		opts.InputType = InputTypeInline
		opts.BodyProvided = true
		inputFlags++
	}
	if cmd.Flags().Changed("body-file") {
		opts.InputType = InputTypeInline
		opts.BodyProvided = true
		inputFlags++
	}
	web, _ := cmd.Flags().GetBool("web")
	if web {
		opts.InputType = InputTypeWeb
		inputFlags++
	}
	if editor, _ := cmd.Flags().GetBool("editor"); editor {
		opts.InputType = InputTypeEditor
		inputFlags++
	}

	if err := cmdutil.MutuallyExclusive(
		"`--attach` is not supported when using `--web`",
		opts.AttachFlag.Changed(),
		web,
	); err != nil {
		return err
	}

	if err := cmdutil.MutuallyExclusive(
		"`--attach` is not supported when using `--delete-last`",
		opts.AttachFlag.Changed(),
		opts.DeleteLast,
	); err != nil {
		return err
	}

	resolved, err := opts.AttachFlag.UserAssets()
	if err != nil {
		return err
	}
	opts.Assets = resolved

	// An asset is a body input on its own, so `--attach shot.png` alone
	// posts an image with no text. It is not part of the mutually exclusive
	// group above, so it only counts when nothing else has.
	if len(opts.Assets) > 0 && inputFlags == 0 {
		opts.InputType = InputTypeInline
		inputFlags++
	}

	if opts.CreateIfNone && !opts.EditLast {
		return cmdutil.FlagErrorf("`--create-if-none` can only be used with `--edit-last`")
	}

	if opts.DeleteLastConfirmed && !opts.DeleteLast {
		return cmdutil.FlagErrorf("`--yes` should only be used with `--delete-last`")
	}

	if opts.DeleteLast {
		if inputFlags > 0 {
			return cmdutil.FlagErrorf("should not provide comment body when using `--delete-last`")
		}
		if opts.IO.CanPrompt() || opts.DeleteLastConfirmed {
			opts.Interactive = opts.IO.CanPrompt()
			return nil
		}
		return cmdutil.FlagErrorf("should provide `--yes` to confirm deletion in non-interactive mode")
	}

	if inputFlags == 0 {
		if !opts.IO.CanPrompt() {
			return cmdutil.FlagErrorf("flags required when not running interactively")
		}
		opts.Interactive = true
	} else if inputFlags > 1 {
		return cmdutil.FlagErrorf("specify only one of `--body`, `--body-file`, `--editor`, or `--web`")
	}

	return nil
}

func CommentableRun(opts *CommentableOptions) error {
	commentable, repo, err := opts.RetrieveCommentable()
	if err != nil {
		return err
	}
	opts.Host = repo.RepoHost()
	if opts.DeleteLast {
		return deleteComment(commentable, opts)
	}

	// Guarded on the assets, because createComment and updateComment return
	// early on the web path. An unguarded check here would fail a run that
	// attached nothing.
	var uploader *attachments.Uploader
	if len(opts.Assets) > 0 {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		// Asked of opts.Host, so the answer is about the host the commentable
		// was actually fetched from.
		tokenType := cfg.Authentication().ActiveTokenType(opts.Host)
		uploader, err = attachments.NewUploader(httpClient, tokenType, opts.Host, commentable.RepositoryDatabaseID(), commentable.RepositoryViewerPermission())
		if err != nil {
			return err
		}
	}

	// Create new comment, bail before complexities of updating the last comment
	if !opts.EditLast {
		return createComment(commentable, opts, uploader)
	}

	// Update the last comment, handling success or unexpected errors accordingly
	err = updateComment(commentable, opts, uploader)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errNoUserComments) {
		return err
	}

	// Determine whether to create new comment, prompt user if interactive and missing option
	if !opts.CreateIfNone && opts.Interactive {
		opts.CreateIfNone, err = opts.ConfirmCreateIfNoneSurvey()
		if err != nil {
			return err
		}
	}
	if !opts.CreateIfNone {
		return errNoUserComments
	}

	// Create new comment because updating the last comment failed due to no user comments
	if opts.Interactive {
		fmt.Fprintln(opts.IO.ErrOut, "No comments found. Creating a new comment.")
	}

	return createComment(commentable, opts, uploader)
}

func createComment(commentable Commentable, opts *CommentableOptions, uploader *attachments.Uploader) error {
	switch opts.InputType {
	case InputTypeWeb:
		openURL := commentable.Link() + "#issuecomment-new"
		if opts.IO.IsStdoutTTY() && !opts.Quiet {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.OpenInBrowser(openURL)
	case InputTypeEditor:
		var body string
		var err error
		if opts.Interactive {
			body, err = opts.InteractiveEditSurvey("")
		} else {
			body, err = opts.EditSurvey("")
		}
		if err != nil {
			return err
		}
		opts.Body = body
	}

	if opts.Interactive {
		cont, err := opts.ConfirmSubmitSurvey()
		if err != nil {
			return err
		}
		if !cont {
			return errors.New("Discarding...")
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	body, write, uploadErr := bodyForWrite(opts, uploader)
	if !write {
		return fmt.Errorf("%w\nno comment was posted", uploadErr)
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	params := api.CommentCreateInput{Body: body, SubjectId: commentable.Identifier()}
	url, err := api.CommentCreate(apiClient, opts.Host, params)
	if err != nil {
		// Upload first: an uploaded asset outlives the failed write, and the
		// user needs to know it exists before hearing the comment did not save.
		return errors.Join(uploadErr, err)
	}

	if !opts.Quiet {
		fmt.Fprintln(opts.IO.Out, url)
	}

	return uploadErr
}

func updateComment(commentable Commentable, opts *CommentableOptions, uploader *attachments.Uploader) error {
	comments := commentable.CurrentUserComments()
	if len(comments) == 0 {
		return errNoUserComments
	}

	lastComment := &comments[len(comments)-1]

	switch opts.InputType {
	case InputTypeWeb:
		openURL := lastComment.Link()
		if opts.IO.IsStdoutTTY() && !opts.Quiet {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.OpenInBrowser(openURL)
	case InputTypeEditor:
		var body string
		var err error
		initialValue := lastComment.Content()
		if opts.Interactive {
			body, err = opts.InteractiveEditSurvey(initialValue)
		} else {
			body, err = opts.EditSurvey(initialValue)
		}
		if err != nil {
			return err
		}
		opts.Body = body
	case InputTypeInline:
		// An explicit `--body ""` still clears the comment. The editor case
		// above is seeded with the comment already, so it needs no merge.
		if opts.KeepExistingBody && !opts.BodyProvided {
			opts.Body = lastComment.Content()
		}
	}

	if opts.Interactive {
		cont, err := opts.ConfirmSubmitSurvey()
		if err != nil {
			return err
		}
		if !cont {
			return errors.New("Discarding...")
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	body, write, uploadErr := bodyForWrite(opts, uploader)
	if !write {
		return fmt.Errorf("%w\nthe comment was not changed", uploadErr)
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	params := api.CommentUpdateInput{Body: body, CommentId: lastComment.Identifier()}
	url, err := api.CommentUpdate(apiClient, opts.Host, params)
	if err != nil {
		// Upload first, for the same reason as createComment.
		return errors.Join(uploadErr, err)
	}

	if !opts.Quiet {
		fmt.Fprintln(opts.IO.Out, url)
	}

	return uploadErr
}

// bodyForWrite uploads the assets and reports whether the body it produced is
// worth writing. A failed upload that produced nothing writes nothing, since
// there is nothing to salvage and a comment that promises an attachment it
// does not have only compounds the failure.
//
// Callers must call this after every prompt, since nothing that can prompt or
// cancel may follow an upload.
func bodyForWrite(opts *CommentableOptions, uploader *attachments.Uploader) (body string, write bool, err error) {
	if uploader == nil {
		return opts.Body, true, nil
	}
	body, uploaded, err := uploader.UploadAndAttach(context.Background(), opts.Body, opts.Assets)
	if err != nil && uploaded == 0 {
		return "", false, err
	}
	return body, true, err
}

func deleteComment(commentable Commentable, opts *CommentableOptions) error {
	comments := commentable.CurrentUserComments()
	if len(comments) == 0 {
		return errNoUserComments
	}

	lastComment := comments[len(comments)-1]

	cs := opts.IO.ColorScheme()

	if opts.Interactive && !opts.DeleteLastConfirmed {
		// This is not an ideal way of truncating a random string that may
		// contain emojis or other kind of wide chars.
		truncated := lastComment.Body
		if len(lastComment.Body) > 40 {
			truncated = lastComment.Body[:40] + "..."
		}

		fmt.Fprintf(opts.IO.Out, "%s Deleted comments cannot be recovered.\n", cs.WarningIcon())
		ok, err := opts.ConfirmDeleteLastComment(truncated)
		if err != nil {
			return err
		}
		if !ok {
			return errDeleteNotConfirmed
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	params := api.CommentDeleteInput{CommentId: lastComment.Identifier()}
	deletionErr := api.CommentDelete(apiClient, opts.Host, params)
	if deletionErr != nil {
		return deletionErr
	}

	if !opts.Quiet {
		fmt.Fprintln(opts.IO.ErrOut, "Comment deleted")
	}

	return nil
}

func CommentableConfirmSubmitSurvey(p Prompt) func() (bool, error) {
	return func() (bool, error) {
		return p.Confirm("Submit?", true)
	}
}

func CommentableInteractiveEditSurvey(cf func() (gh.Config, error), io *iostreams.IOStreams) func(string) (string, error) {
	return func(initialValue string) (string, error) {
		editorCommand, err := cmdutil.DetermineEditor(cf)
		if err != nil {
			return "", err
		}
		cs := io.ColorScheme()
		fmt.Fprintf(io.Out, "- %s to draft your comment in %s... ", cs.Bold("Press Enter"), cs.Bold(surveyext.EditorName(editorCommand)))
		_ = waitForEnter(io.In)
		return surveyext.Edit(editorCommand, "*.md", initialValue, io.In, io.Out, io.ErrOut)
	}
}

func CommentableInteractiveCreateIfNoneSurvey(p Prompt) func() (bool, error) {
	return func() (bool, error) {
		return p.Confirm("No comments found. Create one?", true)
	}
}

func CommentableEditSurvey(cf func() (gh.Config, error), io *iostreams.IOStreams) func(string) (string, error) {
	return func(initialValue string) (string, error) {
		editorCommand, err := cmdutil.DetermineEditor(cf)
		if err != nil {
			return "", err
		}
		return surveyext.Edit(editorCommand, "*.md", initialValue, io.In, io.Out, io.ErrOut)
	}
}

func CommentableConfirmDeleteLastComment(p Prompt) func(string) (bool, error) {
	return func(body string) (bool, error) {
		return p.Confirm(fmt.Sprintf("Delete the comment: %q?", body), true)
	}
}

func waitForEnter(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	return scanner.Err()
}
