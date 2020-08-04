package review

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ReviewOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	SelectorArg    string
	Approve        bool
	RequestChanges bool
	Comment        bool
	Body           string
}

func NewCmdReview(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	opts := &ReviewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "review [<number> | <url> | <branch>]",
		Short: "Add a review to a pull request",
		Long: heredoc.Doc(`
			Add a review to a pull request.

			Without an argument, the pull request that belongs to the current branch is reviewed.
		`),
		Example: heredoc.Doc(`
			# approve the pull request of the current branch
			$ gh pr review --approve
			
			# leave a review comment for the current branch
			$ gh pr review --comment -b "interesting"
			
			# add a review for a specific pull request
			$ gh pr review 123
			
			# request changes on a specific pull request
			$ gh pr review 123 -r -b "needs more ASCII art"
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return reviewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Approve, "approve", "a", false, "Approve pull request")
	cmd.Flags().BoolVarP(&opts.RequestChanges, "request-changes", "r", false, "Request changes on a pull request")
	cmd.Flags().BoolVarP(&opts.Comment, "comment", "c", false, "Comment on a pull request")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Specify the body of a review")

	return cmd
}

func reviewRun(opts *ReviewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	reviewData, err := processReviewOpt(opts)
	if err != nil {
		return fmt.Errorf("did not understand desired review action: %w", err)
	}

	pr, baseRepo, err := shared.PRFromArgs(apiClient, opts.BaseRepo, opts.Branch, opts.Remotes, opts.SelectorArg)
	if err != nil {
		return err
	}

	if reviewData == nil {
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}
		reviewData, err = reviewSurvey(opts.IO, editorCommand)
		if err != nil {
			return err
		}
		if reviewData == nil && err == nil {
			fmt.Fprint(opts.IO.ErrOut, "Discarding.\n")
			return nil
		}
	}

	err = api.AddReview(apiClient, baseRepo, pr, reviewData)
	if err != nil {
		return fmt.Errorf("failed to create review: %w", err)
	}

	if !opts.IO.IsStdoutTTY() || !opts.IO.IsStderrTTY() {
		return nil
	}

	switch reviewData.State {
	case api.ReviewComment:
		fmt.Fprintf(opts.IO.ErrOut, "%s Reviewed pull request #%d\n", utils.Gray("-"), pr.Number)
	case api.ReviewApprove:
		fmt.Fprintf(opts.IO.ErrOut, "%s Approved pull request #%d\n", utils.Green("✓"), pr.Number)
	case api.ReviewRequestChanges:
		fmt.Fprintf(opts.IO.ErrOut, "%s Requested changes to pull request #%d\n", utils.Red("+"), pr.Number)
	}

	return nil
}

// TODO: move to Command.Args, raise FlagError
func processReviewOpt(opts *ReviewOptions) (*api.PullRequestReviewInput, error) {
	found := 0
	flag := ""
	var state api.PullRequestReviewState

	if opts.Approve {
		found++
		flag = "approve"
		state = api.ReviewApprove
	}
	if opts.RequestChanges {
		found++
		flag = "request-changes"
		state = api.ReviewRequestChanges
	}
	if opts.Comment {
		found++
		flag = "comment"
		state = api.ReviewComment
	}

	body := opts.Body

	if found == 0 && body == "" {
		if opts.IO.IsStdoutTTY() && opts.IO.IsStderrTTY() {
			return nil, nil // signal interactive mode
		}
		return nil, errors.New("--approve, --request-changes, or --comment required when not attached to a tty")
	} else if found == 0 && body != "" {
		return nil, errors.New("--body unsupported without --approve, --request-changes, or --comment")
	} else if found > 1 {
		return nil, errors.New("need exactly one of --approve, --request-changes, or --comment")
	}

	if (flag == "request-changes" || flag == "comment") && body == "" {
		return nil, fmt.Errorf("body cannot be blank for %s review", flag)
	}

	return &api.PullRequestReviewInput{
		Body:  body,
		State: state,
	}, nil
}

func reviewSurvey(io *iostreams.IOStreams, editorCommand string) (*api.PullRequestReviewInput, error) {
	typeAnswers := struct {
		ReviewType string
	}{}
	typeQs := []*survey.Question{
		{
			Name: "reviewType",
			Prompt: &survey.Select{
				Message: "What kind of review do you want to give?",
				Options: []string{
					"Comment",
					"Approve",
					"Request changes",
				},
			},
		},
	}

	err := prompt.SurveyAsk(typeQs, &typeAnswers)
	if err != nil {
		return nil, err
	}

	var reviewState api.PullRequestReviewState

	switch typeAnswers.ReviewType {
	case "Approve":
		reviewState = api.ReviewApprove
	case "Request changes":
		reviewState = api.ReviewRequestChanges
	case "Comment":
		reviewState = api.ReviewComment
	default:
		panic("unreachable state")
	}

	bodyAnswers := struct {
		Body string
	}{}

	blankAllowed := false
	if reviewState == api.ReviewApprove {
		blankAllowed = true
	}

	bodyQs := []*survey.Question{
		{
			Name: "body",
			Prompt: &surveyext.GhEditor{
				BlankAllowed:  blankAllowed,
				EditorCommand: editorCommand,
				Editor: &survey.Editor{
					Message:  "Review body",
					FileName: "*.md",
				},
			},
		},
	}

	err = prompt.SurveyAsk(bodyQs, &bodyAnswers)
	if err != nil {
		return nil, err
	}

	if bodyAnswers.Body == "" && (reviewState == api.ReviewComment || reviewState == api.ReviewRequestChanges) {
		return nil, errors.New("this type of review cannot be blank")
	}

	if len(bodyAnswers.Body) > 0 {
		renderedBody, err := utils.RenderMarkdown(bodyAnswers.Body)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(io.Out, "Got:\n%s", renderedBody)
	}

	confirm := false
	confirmQs := []*survey.Question{
		{
			Name: "confirm",
			Prompt: &survey.Confirm{
				Message: "Submit?",
				Default: true,
			},
		},
	}

	err = prompt.SurveyAsk(confirmQs, &confirm)
	if err != nil {
		return nil, err
	}

	if !confirm {
		return nil, nil
	}

	return &api.PullRequestReviewInput{
		Body:  bodyAnswers.Body,
		State: reviewState,
	}, nil
}
