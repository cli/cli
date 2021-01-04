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
	editor = iota
	web
	inline
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
			$ gh issue comment 22 --body "I was able to reproduce this issue, lets fix it."
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.SelectorArg = args[0]

			inputFlags := 0
			if cmd.Flags().Changed("body") {
				opts.InputType = inline
				inputFlags++
			}
			if webMode {
				opts.InputType = web
				inputFlags++
			}
			if editorMode {
				opts.InputType = editor
				inputFlags++
			}

			if inputFlags == 0 {
				if !opts.IO.CanPrompt() {
					return &cmdutil.FlagError{Err: errors.New("--body or --web required when not running interactively")}
				}
				opts.Interactive = true
			} else if inputFlags == 1 {
				if !opts.IO.CanPrompt() && opts.InputType == editor {
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
	case editor:
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}
		body, err := opts.Edit(editorCommand)
		if err != nil {
			return err
		}
		opts.Body = body
		if opts.Interactive {
			cs := opts.IO.ColorScheme()
			fmt.Fprint(opts.IO.Out, cs.GreenBold("?"))
			fmt.Fprint(opts.IO.Out, cs.Bold(" Body "))
			fmt.Fprint(opts.IO.Out, cs.Cyan("<Received>\n"))
		}
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
	fmt.Fprintln(opts.IO.Out, url)
	return nil
}

func inputTypeSurvey() (int, error) {
	var inputType int
	inputTypeQuestion := &survey.Select{
		Message: "Where do you want to draft your comment?",
		Options: []string{"Editor", "Web"},
	}
	err := prompt.SurveyAskOne(inputTypeQuestion, &inputType)
	return inputType, err
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
