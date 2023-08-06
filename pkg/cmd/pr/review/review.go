package review

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
	"github.com/waigani/diffparser"
)

type ReviewOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Prompter   prompter.Prompter

	Finder shared.PRFinder

	SelectorArg     string
	InteractiveMode bool
	ReviewType      api.PullRequestReviewState
	Body            string
	CreateSuggestions bool
}

func NewCmdReview(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	opts := &ReviewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
	}

	var (
		flagApprove        bool
		flagRequestChanges bool
		flagComment        bool
		flagSuggest        bool
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

			# suggest changes to the current pull request
			$ git add path/to/file/with/changes
			$ gh pr review -s
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

			if flagSuggest {
				opts.CreateSuggestions = true
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
	cmd.Flags().BoolVarP(&flagSuggest, "suggest", "s", false, "Make suggestions on a pull request")
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

	createReviewSuggestions(opts, baseRepo, pr)

	var reviewData *api.PullRequestReviewInput
	if opts.InteractiveMode {
		editorCommand, err := cmdutil.DetermineEditor(opts.Config)
		if err != nil {
			return err
		}
		reviewData, err = reviewSurvey(opts, editorCommand)
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

func reviewSurvey(opts *ReviewOptions, editorCommand string) (*api.PullRequestReviewInput, error) {
	options := []string{"Comment", "Approve", "Request Changes"}
	reviewType, err := opts.Prompter.Select(
		"What kind of review do you want to give?",
		options[0],
		options)
	if err != nil {
		return nil, err
	}

	var reviewState api.PullRequestReviewState

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

	blankAllowed := false
	if reviewState == api.ReviewApprove {
		blankAllowed = true
	}

	body, err := opts.Prompter.MarkdownEditor("Review body", "", blankAllowed)
	if err != nil {
		return nil, err
	}

	if body == "" && (reviewState == api.ReviewComment || reviewState == api.ReviewRequestChanges) {
		return nil, errors.New("this type of review cannot be blank")
	}

	if len(body) > 0 {
		renderedBody, err := markdown.Render(body,
			markdown.WithTheme(opts.IO.TerminalTheme()),
			markdown.WithWrap(opts.IO.TerminalWidth()))
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(opts.IO.Out, "Got:\n%s", renderedBody)
	}

	confirm, err := opts.Prompter.Confirm("Submit?", true)
	if err != nil {
		return nil, err
	}

	if !confirm {
		return nil, nil
	}

	return &api.PullRequestReviewInput{
		Body:  body,
		State: reviewState,
	}, nil
}

func createReviewSuggestions(opts *ReviewOptions, baseRepo ghrepo.Interface, pr *api.PullRequest) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	gitClient := opts.GitClient
	stagedDiffString, err := gitClient.StagedDiff(context.Background())
	if err != nil {
		return fmt.Errorf("failed to compute staged diff: %w", err)
	}

	stagedDiff, err := diffparser.Parse(stagedDiffString)
	if err != nil {
		return fmt.Errorf("failed to parse staged diff: %w", err)
	}

	for _, file := range stagedDiff.Files {
		origFileName := file.OrigName

		for _, hunk := range file.Hunks {
			origStart := hunk.OrigRange.Start
			origLength := hunk.OrigRange.Length
			reviewThreadBody := makeSuggestionBody(hunk)

			reviewThreadData := &api.PullRequestReviewThreadInput{
				Body: reviewThreadBody,
				Path: origFileName,
			}

			// The origStart == origLength is a hack to get around a bug in diffparser
			// which doesn't correctly recognize one line diffs. This could absolutely
			// create false positives, so it needs to be fixed properly upstream at
			// some point.
			if origLength == 1 || origStart == origLength {
				reviewThreadData.Line = origStart
			} else {
				reviewThreadData.StartLine = origStart
				reviewThreadData.Line = origStart + origLength - 1
			}

			err = api.AddReviewThread(apiClient, baseRepo, pr, reviewThreadData)
			if err != nil {
				return fmt.Errorf("failed to create review thread: %w", err)
			}
		}
	}

	return nil
}

func makeSuggestionBody(hunk *diffparser.DiffHunk) string {
	bodyString := "```suggestion\n"
	for _, line := range hunk.NewRange.Lines {
		bodyString += line.Content + "\n"
	}
	return bodyString + "```"
}
