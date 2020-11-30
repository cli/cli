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
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CommentOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Edit       func(string) (string, error)

	SelectorArg string
	Interactive bool
	InputType   int
	Body        string
}

const (
	inline = iota
	editor
	web
)

func NewCmdComment(f *cmdutil.Factory, runF func(*CommentOptions) error) *cobra.Command {
	opts := &CommentOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Edit: func(editorCommand string) (string, error) {
			return surveyext.Edit(editorCommand, "*.md", "", f.IOStreams.In, f.IOStreams.Out, f.IOStreams.ErrOut, nil)
		},
	}

	var webMode bool
	var editorMode bool

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Create a new issue comment",
		Example: heredoc.Doc(`
			$ gh issue comment 22 --body "I found a bug. Nothing works"
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.SelectorArg = args[0]
			opts.Interactive = !(opts.Body != "" || webMode || editorMode)

			if !opts.IO.CanPrompt() && opts.Interactive {
				return &cmdutil.FlagError{Err: errors.New("--body, --editor, or --web required when not running interactively")}
			}

			if !opts.Interactive {
				if opts.Body != "" && !webMode && !editorMode {
					opts.InputType = inline
				} else if opts.Body == "" && webMode && !editorMode {
					opts.InputType = web
				} else if opts.Body == "" && !webMode && editorMode {
					opts.InputType = editor
				} else {
					return &cmdutil.FlagError{Err: fmt.Errorf("specify only one of --body, --editor, or --web")}
				}
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
		inputType, err := inputTypeSurvey()
		if err != nil {
			return err
		}
		opts.InputType = inputType
	}

	switch opts.InputType {
	case web:
		openURL := issue.URL + "#issuecomment-new"
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(openURL))
		}
		return utils.OpenInBrowser(openURL)
	case inline:
		if opts.Body != "" {
			break
		}
		body, err := inlineSurvey()
		if err != nil {
			return err
		}
		opts.Body = body
	case editor:
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}
		var body string
		if opts.Interactive {
			body, err = editorSurvey(editorCommand)
		} else {
			body, err = opts.Edit(editorCommand)
		}
		if err != nil {
			return err
		}
		opts.Body = body
	}

	if opts.Interactive {
		cont, err := confirmSubmitSurvey()
		if err != nil {
			return err
		}
		if !cont {
			return fmt.Errorf("Discarding...")
		}
	}

	params := map[string]string{
		"subjectId": issue.ID,
		"body":      opts.Body,
	}

	url, err := api.CommentCreate(apiClient, baseRepo.RepoHost(), params)
	if err != nil {
		return err
	}

	fmt.Fprint(opts.IO.Out, url)

	return nil
}

func inputTypeSurvey() (int, error) {
	var inputType int
	inputTypeQuestion := &survey.Select{
		Message: "Where do you want to draft your comment?",
		Options: []string{"In line", "Editor", "Web"},
	}
	err := prompt.SurveyAskOne(inputTypeQuestion, &inputType)
	return inputType, err
}

func inlineSurvey() (string, error) {
	var body string
	bodyQuestion := &survey.Input{
		Message: "Body",
	}
	err := prompt.SurveyAskOne(bodyQuestion, &body)
	return body, err
}

func editorSurvey(editorCommand string) (string, error) {
	var body string
	bodyQuestion := &surveyext.GhEditor{
		BlankAllowed:  false,
		EditorCommand: editorCommand,
		Editor: &survey.Editor{
			Message:  "Body",
			FileName: "*.md",
		},
	}
	err := prompt.SurveyAskOne(bodyQuestion, &body)
	return body, err
}

func confirmSubmitSurvey() (bool, error) {
	var confirm bool
	submit := &survey.Confirm{
		Message: "Submit?",
		Default: true,
	}
	err := prompt.SurveyAskOne(submit, &confirm)
	return confirm, err
}
