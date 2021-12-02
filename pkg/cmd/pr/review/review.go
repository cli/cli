package review

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/pkg/surveyext"
	"github.com/spf13/cobra"
)

type ReviewOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg     string
	InteractiveMode bool
	ReviewType      api.PullRequestReviewState
	Body            string
}

func NewCmdReview(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	opts := &ReviewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	var (
		flagApprove        bool
		flagRequestChanges bool
		flagComment        bool
	)

	var bodyFile string

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
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			bodyProvided := cmd.Flags().Changed("body")
			bodyFileProvided := bodyFile != ""

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--body` or `--body-file`",
				bodyProvided,
				bodyFileProvided,
			); err != nil {
				return err
			}
			if bodyFileProvided {
				b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(b)
			}

			found := 0
			if flagApprove {
				found++
				opts.ReviewType = api.ReviewApprove
			}
			if flagRequestChanges {
				found++
				opts.ReviewType = api.ReviewRequestChanges
				if opts.Body == "" {
					return cmdutil.FlagErrorf("body cannot be blank for request-changes review")
				}
			}
			if flagComment {
				found++
				opts.ReviewType = api.ReviewComment
				if opts.Body == "" {
					return cmdutil.FlagErrorf("body cannot be blank for comment review")
				}
			}

			if found == 0 && opts.Body == "" {
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("--approve, --request-changes, or --comment required when not running interactively")
				}
				opts.InteractiveMode = true
			} else if found == 0 && opts.Body != "" {
				return cmdutil.FlagErrorf("--body unsupported without --approve, --request-changes, or --comment")
			} else if found > 1 {
				return cmdutil.FlagErrorf("need exactly one of --approve, --request-changes, or --comment")
			}

			if runF != nil {
				return runF(opts)
			}
			return reviewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&flagApprove, "approve", "a", false, "Approve pull request")
	cmd.Flags().BoolVarP(&flagRequestChanges, "request-changes", "r", false, "Request changes on a pull request")
	cmd.Flags().BoolVarP(&flagComment, "comment", "c", false, "Comment on a pull request")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Specify the body of a review")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")

	return cmd
}

func reviewRun(opts *ReviewOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "number"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	var reviewData *api.PullRequestReviewInput
	if opts.InteractiveMode {
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
	} else {
		reviewData = &api.PullRequestReviewInput{
			State: opts.ReviewType,
			Body:  opts.Body,
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	err = api.AddReview(apiClient, baseRepo, pr, reviewData)
	if err != nil {
		return fmt.Errorf("failed to create review: %w", err)
	}

	if !opts.IO.IsStdoutTTY() || !opts.IO.IsStderrTTY() {
		return nil
	}

	cs := opts.IO.ColorScheme()

	switch reviewData.State {
	case api.ReviewComment:
		fmt.Fprintf(opts.IO.ErrOut, "%s Reviewed pull request #%d\n", cs.Gray("-"), pr.Number)
	case api.ReviewApprove:
		fmt.Fprintf(opts.IO.ErrOut, "%s Approved pull request #%d\n", cs.SuccessIcon(), pr.Number)
	case api.ReviewRequestChanges:
		fmt.Fprintf(opts.IO.ErrOut, "%s Requested changes to pull request #%d\n", cs.Red("+"), pr.Number)
	}

	return nil
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
		style := markdown.GetStyle(io.DetectTerminalTheme())
		renderedBody, err := markdown.Render(bodyAnswers.Body, style)
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
