package shared

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type InputType int

const (
	InputTypeEditor InputType = iota
	InputTypeInline
	InputTypeWeb
)

type Commentable interface {
	Link() string
	Identifier() string
}

type CommentableOptions struct {
	IO                    *iostreams.IOStreams
	HttpClient            func() (*http.Client, error)
	RetrieveCommentable   func() (Commentable, ghrepo.Interface, error)
	EditSurvey            func() (string, error)
	InteractiveEditSurvey func() (string, error)
	ConfirmSubmitSurvey   func() (bool, error)
	OpenInBrowser         func(string) error
	Interactive           bool
	InputType             InputType
	Body                  string
}

func CommentablePreRun(cmd *cobra.Command, opts *CommentableOptions) error {
	inputFlags := 0
	if cmd.Flags().Changed("body") {
		opts.InputType = InputTypeInline
		inputFlags++
	}
	if cmd.Flags().Changed("body-file") {
		opts.InputType = InputTypeInline
		inputFlags++
	}
	if web, _ := cmd.Flags().GetBool("web"); web {
		opts.InputType = InputTypeWeb
		inputFlags++
	}
	if editor, _ := cmd.Flags().GetBool("editor"); editor {
		opts.InputType = InputTypeEditor
		inputFlags++
	}

	if inputFlags == 0 {
		if !opts.IO.CanPrompt() {
			return &cmdutil.FlagError{Err: errors.New("`--body`, `--body-file` or `--web` required when not running interactively")}
		}
		opts.Interactive = true
	} else if inputFlags == 1 {
		if !opts.IO.CanPrompt() && opts.InputType == InputTypeEditor {
			return &cmdutil.FlagError{Err: errors.New("`--body`, `--body-file` or `--web` required when not running interactively")}
		}
	} else if inputFlags > 1 {
		return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of `--body`, `--body-file`, `--editor`, or `--web`")}
	}

	return nil
}

func CommentableRun(opts *CommentableOptions) error {
	commentable, repo, err := opts.RetrieveCommentable()
	if err != nil {
		return err
	}

	switch opts.InputType {
	case InputTypeWeb:
		openURL := commentable.Link() + "#issuecomment-new"
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return opts.OpenInBrowser(openURL)
	case InputTypeEditor:
		var body string
		if opts.Interactive {
			body, err = opts.InteractiveEditSurvey()
		} else {
			body, err = opts.EditSurvey()
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
	apiClient := api.NewClientFromHTTP(httpClient)
	params := api.CommentCreateInput{Body: opts.Body, SubjectId: commentable.Identifier()}
	url, err := api.CommentCreate(apiClient, repo.RepoHost(), params)
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.IO.Out, url)
	return nil
}

func CommentableConfirmSubmitSurvey() (bool, error) {
	var confirm bool
	submit := &survey.Confirm{
		Message: "Submit?",
		Default: true,
	}
	err := survey.AskOne(submit, &confirm)
	return confirm, err
}

func CommentableInteractiveEditSurvey(cf func() (config.Config, error), io *iostreams.IOStreams) func() (string, error) {
	return func() (string, error) {
		editorCommand, err := cmdutil.DetermineEditor(cf)
		if err != nil {
			return "", err
		}
		if editorCommand == "" {
			editorCommand = surveyext.DefaultEditorName()
		}
		cs := io.ColorScheme()
		fmt.Fprintf(io.Out, "- %s to draft your comment in %s... ", cs.Bold("Press Enter"), cs.Bold(editorCommand))
		_ = waitForEnter(io.In)
		return surveyext.Edit(editorCommand, "*.md", "", io.In, io.Out, io.ErrOut, nil)
	}
}

func CommentableEditSurvey(cf func() (config.Config, error), io *iostreams.IOStreams) func() (string, error) {
	return func() (string, error) {
		editorCommand, err := cmdutil.DetermineEditor(cf)
		if err != nil {
			return "", err
		}
		return surveyext.Edit(editorCommand, "*.md", "", io.In, io.Out, io.ErrOut, nil)
	}
}

func waitForEnter(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Scan()
	return scanner.Err()
}
