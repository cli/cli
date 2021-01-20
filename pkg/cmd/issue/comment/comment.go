package comment

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/issue/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CommentOptions struct {
	HttpClient          func() (*http.Client, error)
	IO                  *iostreams.IOStreams
	BaseRepo            func() (ghrepo.Interface, error)
	EditSurvey          func() (string, error)
	InputTypeSurvey     func() (inputType, error)
	ConfirmSubmitSurvey func() (bool, error)
	OpenInBrowser       func(string) error

	SelectorArg string
	Interactive bool
	InputType   inputType
	Body        string
}

type inputType int

const (
	inputTypeEditor inputType = iota
	inputTypeInline
	inputTypeWeb
)

func NewCmdComment(f *cmdutil.Factory, runF func(*CommentOptions) error) *cobra.Command {
	opts := &CommentOptions{
		IO:                  f.IOStreams,
		HttpClient:          f.HttpClient,
		EditSurvey:          editSurvey(f.Config, f.IOStreams),
		InputTypeSurvey:     inputTypeSurvey,
		ConfirmSubmitSurvey: confirmSubmitSurvey,
		OpenInBrowser:       utils.OpenInBrowser,
	}

	var webMode bool
	var editorMode bool

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Create a new issue comment",
		Example: heredoc.Doc(`
			$ gh issue comment 22 --body "I was able to reproduce this issue, lets fix it."
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.SelectorArg = args[0]

			inputFlags := 0
			if cmd.Flags().Changed("body") {
				opts.InputType = inputTypeInline
				inputFlags++
			}
			if webMode {
				opts.InputType = inputTypeWeb
				inputFlags++
			}
			if editorMode {
				opts.InputType = inputTypeEditor
				inputFlags++
			}

			if inputFlags == 0 {
				if !opts.IO.CanPrompt() {
					return &cmdutil.FlagError{Err: errors.New("--body or --web required when not running interactively")}
				}
				opts.Interactive = true
			} else if inputFlags == 1 {
				if !opts.IO.CanPrompt() && opts.InputType == inputTypeEditor {
					return &cmdutil.FlagError{Err: errors.New("--body or --web required when not running interactively")}
				}
			} else if inputFlags > 1 {
				return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of --body, --editor, or --web")}
			}

			if runF != nil {
				return runF(opts)
			}
			return commentRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	cmd.Flags().BoolVarP(&editorMode, "editor", "e", false, "Add body using editor")
	cmd.Flags().BoolVarP(&webMode, "web", "w", false, "Add body in browser")

	return cmd
}

func commentRun(opts *CommentOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	issue, baseRepo, err := shared.IssueFromArg(apiClient, opts.BaseRepo, opts.SelectorArg)
	if err != nil {
		return err
	}

	if opts.Interactive {
		inputType, err := opts.InputTypeSurvey()
		if err != nil {
			return err
		}
		opts.InputType = inputType
	}

	switch opts.InputType {
	case inputTypeWeb:
		openURL := issue.URL + "#issuecomment-new"
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return opts.OpenInBrowser(openURL)
	case inputTypeEditor:
		body, err := opts.EditSurvey()
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
			return fmt.Errorf("Discarding...")
		}
	}

	params := api.CommentCreateInput{Body: opts.Body, SubjectId: issue.ID}
	url, err := api.CommentCreate(apiClient, baseRepo.RepoHost(), params)
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.IO.Out, url)
	return nil
}

var inputTypeSurvey = func() (inputType, error) {
	var result int
	inputTypeQuestion := &survey.Select{
		Message: "Where do you want to draft your comment?",
		Options: []string{"Editor", "Web"},
	}
	err := survey.AskOne(inputTypeQuestion, &result)
	if err != nil {
		return 0, err
	}

	if result == 0 {
		return inputTypeEditor, nil
	} else {
		return inputTypeWeb, nil
	}
}

var confirmSubmitSurvey = func() (bool, error) {
	var confirm bool
	submit := &survey.Confirm{
		Message: "Submit?",
		Default: true,
	}
	err := survey.AskOne(submit, &confirm)
	return confirm, err
}

var editSurvey = func(cf func() (config.Config, error), io *iostreams.IOStreams) func() (string, error) {
	return func() (string, error) {
		editorCommand, err := cmdutil.DetermineEditor(cf)
		if err != nil {
			return "", err
		}
		return surveyext.Edit(editorCommand, "*.md", "", io.In, io.Out, io.ErrOut, nil)
	}
}
