package review

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

type ReviewOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Prompter   prompter.Prompter

	Finder shared.PRFinder

	SelectorArg     string
	InteractiveMode bool
	ReviewType      api.PullRequestReviewState
	Body            string
	File            string
	Line            int
	StartLine       *int
}

func NewCmdReview(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	opts := &ReviewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
	}

	var (
		flagApprove        bool
		flagRequestChanges bool
		flagComment        bool
	)

	var bodyFile string
	var linesSlice []int

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

			# leave a comment on specific lines in a file
			$ gh pr review --file README.md --lines 5,6 --body "this is great"
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

			if opts.File != 0 && found > 0 {
				return cmdutil.FlagErrorf("cannot specify --approve, --request-changes, or --comment when commenting on a file")
			}

			if len(linesSlice) > 0 && opts.File == "" {
				return cmdutil.FlagErrorf("must specify --file if --lines is given")
			}

			if len(linesSlice) == 1 {
				opts.Line = linesSlice[0]
			} else if len(linesSlice) == 2 {
				opts.StartLine = &linesSlice[0]
				opts.Line = linesSlice[1]
			} else {
				return cmdutil.FlagErrorf("--lines must be either a single line, or a range")
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
	cmd.Flags().StringVarP(&opts.File, "file", "f", "", "Specify the file to which the comment pertains")
	cmd.Flags().IntSliceVarP(&linesSlice, "lines", "l", []int{}, "Specify the lines in `file` to which the comment pertains")

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

	var body string
	var reviewState *api.PullRequestReviewState
	if opts.InteractiveMode {
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}
		body, reviewState, err = reviewSurvey(opts, editorCommand)
		if err != nil {
			return err
		}
		if reviewState == nil && err == nil {
			fmt.Fprint(opts.IO.ErrOut, "Discarding.\n")
			return nil
		}
	} else {
		body = opts.Body
		reviewState = &opts.ReviewType
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	if opts.File == "" {
		reviewData := &api.PullRequestReviewInput{
			State: *reviewState,
			Body:  body,
		}
		err = api.AddReview(apiClient, baseRepo, pr, reviewData)
		if err != nil {
			return fmt.Errorf("failed to create review: %w", err)
		}

		if !opts.IO.IsStdoutTTY() || !opts.IO.IsStderrTTY() {
			return nil
		}

		cs := opts.IO.ColorScheme()

		switch *reviewState {
		case api.ReviewComment:
			fmt.Fprintf(opts.IO.ErrOut, "%s Reviewed pull request #%d\n", cs.Gray("-"), pr.Number)
		case api.ReviewApprove:
			fmt.Fprintf(opts.IO.ErrOut, "%s Approved pull request #%d\n", cs.SuccessIcon(), pr.Number)
		case api.ReviewRequestChanges:
			fmt.Fprintf(opts.IO.ErrOut, "%s Requested changes to pull request #%d\n", cs.Red("+"), pr.Number)
		}
	} else {
		reviewThreadData := &api.PullRequestReviewThreadInput{
			Body:      body,
			Path:      opts.File,
			Line:      opts.Line,
			StartLine: opts.StartLine,
		}
		err = api.AddReviewThread(apiClient, baseRepo, pr, reviewThreadData)
		if err != nil {
			return fmt.Errorf("failed to create review thread: %w", err)
		}

		if !opts.IO.IsStdoutTTY() || !opts.IO.IsStderrTTY() {
			return nil
		}
	}

	return nil
}

func reviewSurvey(opts *ReviewOptions, editorCommand string) (string, *api.PullRequestReviewState, error) {
	var reviewState api.PullRequestReviewState
	if opts.File == "" {
		options := []string{"Comment", "Approve", "Request Changes"}
		reviewType, err := opts.Prompter.Select(
			"What kind of review do you want to give?",
			options[0],
			options)
		if err != nil {
			return "", nil, err
		}

		switch reviewType {
		case 0:
			reviewState = api.ReviewComment
		case 1:
			reviewState = api.ReviewApprove
		case 2:
			reviewState = api.ReviewRequestChanges
		default:
			panic("unreachable state")
		}
	} else {
		reviewState = api.ReviewComment
	}

	blankAllowed := false
	if reviewState == api.ReviewApprove {
		blankAllowed = true
	}

	body, err := opts.Prompter.MarkdownEditor("Review body", "", blankAllowed)
	if err != nil {
		return "", nil, err
	}

	if body == "" && (reviewState == api.ReviewComment || reviewState == api.ReviewRequestChanges) {
		return "", nil, errors.New("this type of review cannot be blank")
	}

	if len(body) > 0 {
		renderedBody, err := markdown.Render(body,
			markdown.WithTheme(opts.IO.TerminalTheme()),
			markdown.WithWrap(opts.IO.TerminalWidth()))
		if err != nil {
			return "", nil, err
		}

		fmt.Fprintf(opts.IO.Out, "Got:\n%s", renderedBody)
	}

	confirm, err := opts.Prompter.Confirm("Submit?", true)
	if err != nil {
		return "", nil, err
	}

	if !confirm {
		return "", nil, nil
	}

	return body, &reviewState, nil
}
