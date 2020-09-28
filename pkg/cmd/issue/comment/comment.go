package comment

import (
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
	"github.com/spf13/cobra"
)

type CommentOptions struct {
	HttpClient  func() (*http.Client, error)
	Config      func() (config.Config, error)
	IO          *iostreams.IOStreams
	BaseRepo    func() (ghrepo.Interface, error)
	Body        string
	SelectorArg string
	Interactive bool
	Action      Action
}

func NewCmdComment(f *cmdutil.Factory, runF func(*CommentOptions) error) *cobra.Command {
	opts := &CommentOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Create comments for the issue",
		Example: heredoc.Doc(`
			$ gh issue comment --body "I found a bug. Nothing works"
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			bodyProvided := cmd.Flags().Changed("body")

			opts.Interactive = !(bodyProvided)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return commentRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body.")

	return cmd
}

type Action int

const (
	SubmitAction Action = iota
	CancelAction
)

func titleBodySurvey(editorCommand string, issueState *CommentOptions, apiClient *api.Client, repo ghrepo.Interface, providedBody string) error {
	bodyQuestion := &survey.Question{
		Name: "body",
		Prompt: &surveyext.GhEditor{
			BlankAllowed:  false,
			EditorCommand: editorCommand,
			Editor: &survey.Editor{
				Message:  "Body",
				FileName: "*.md",
				Default:  issueState.Body,
			},
		},
	}

	var qs []*survey.Question

	if providedBody == "" {
		qs = append(qs, bodyQuestion)
	}

	err := prompt.SurveyAsk(qs, issueState)
	if err != nil {
		panic(fmt.Sprintf("could not prompt: %w", err))
	}

	confirmA, err := confirmSubmission()

	if err != nil {
		panic(fmt.Sprintf("unable to confirm: %w", err))
	}

	issueState.Action = confirmA
	return nil
}

func confirmSubmission() (Action, error) {
	const (
		submitLabel = "Submit"
		cancelLabel = "Cancel"
	)

	options := []string{submitLabel, cancelLabel}

	confirmAnswers := struct {
		Confirmation int
	}{}
	confirmQs := []*survey.Question{
		{
			Name: "confirmation",
			Prompt: &survey.Select{
				Message: "What's next?",
				Options: options,
			},
		},
	}

	err := prompt.SurveyAsk(confirmQs, &confirmAnswers)
	if err != nil {
		return -1, fmt.Errorf("could not prompt: %w", err)
	}

	switch options[confirmAnswers.Confirmation] {
	case submitLabel:
		return SubmitAction, nil
	case cancelLabel:
		return CancelAction, nil
	default:
		return -1, fmt.Errorf("invalid index: %d", confirmAnswers.Confirmation)
	}
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

	isTerminal := opts.IO.IsStdoutTTY()

	if isTerminal {
		fmt.Fprintf(opts.IO.ErrOut, "\nMake a comment for %s in %s\n\n", issue.Title, ghrepo.FullName(baseRepo))
	}

	action := SubmitAction
	body := opts.Body

	if opts.Interactive {
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}

		err = titleBodySurvey(editorCommand, opts, apiClient, baseRepo, body)

		if err != nil {
			return err
		}

		action = opts.Action
	}

	if action == CancelAction {
		return nil
	} else if action == SubmitAction {
		params := map[string]interface{}{
			"subjectId": issue.ID,
			"body":      opts.Body,
		}

		err = api.CommentCreate(apiClient, baseRepo.RepoHost(), params)
		if err != nil {
			return err
		}

		fmt.Fprintln(opts.IO.Out, issue.URL)

		return nil
	}
	return fmt.Errorf("unexpected action state: %v", action)
}
