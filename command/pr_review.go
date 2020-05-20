package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/cli/cli/utils"
)

func init() {
	prCmd.AddCommand(prReviewCmd)

	prReviewCmd.Flags().BoolP("approve", "a", false, "Approve pull request")
	prReviewCmd.Flags().BoolP("request-changes", "r", false, "Request changes on a pull request")
	prReviewCmd.Flags().BoolP("comment", "c", false, "Comment on a pull request")
	prReviewCmd.Flags().StringP("body", "b", "", "Specify the body of a review")
}

var prReviewCmd = &cobra.Command{
	Use:   "review [{<number> | <url> | <branch>]",
	Short: "Add a review to a pull request.",
	Args:  cobra.MaximumNArgs(1),
	Long: `Add a review to either a specified pull request or the pull request associated with the current branch.

Examples:

	gh pr review                                  # add a review for the current branch's pull request
	gh pr review 123                              # add a review for pull request 123
	gh pr review -a                               # mark the current branch's pull request as approved
	gh pr review -c -b "interesting"              # comment on the current branch's pull request
	gh pr review 123 -r -b "needs more ascii art" # request changes on pull request 123
	`,
	RunE: prReview,
}

func processReviewOpt(cmd *cobra.Command) (*api.PullRequestReviewInput, error) {
	found := 0
	flag := ""
	var state api.PullRequestReviewState

	if cmd.Flags().Changed("approve") {
		found++
		flag = "approve"
		state = api.ReviewApprove
	}
	if cmd.Flags().Changed("request-changes") {
		found++
		flag = "request-changes"
		state = api.ReviewRequestChanges
	}
	if cmd.Flags().Changed("comment") {
		found++
		flag = "comment"
		state = api.ReviewComment
	}

	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return nil, err
	}

	if found == 0 && body == "" {
		return nil, nil // signal interactive mode
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

func prReview(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	var prNum int
	branchWithOwner := ""

	if len(args) == 0 {
		prNum, branchWithOwner, err = prSelectorForCurrentBranch(ctx, baseRepo)
		if err != nil {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}
	} else {
		prArg, repo := prFromURL(args[0])
		if repo != nil {
			baseRepo = repo
		} else {
			prArg = strings.TrimPrefix(args[0], "#")
		}
		prNum, err = strconv.Atoi(prArg)
		if err != nil {
			return errors.New("could not parse pull request argument")
		}
	}

	reviewData, err := processReviewOpt(cmd)
	if err != nil {
		return fmt.Errorf("did not understand desired review action: %w", err)
	}

	var pr *api.PullRequest
	if prNum > 0 {
		pr, err = api.PullRequestByNumber(apiClient, baseRepo, prNum)
		if err != nil {
			return fmt.Errorf("could not find pull request: %w", err)
		}
	} else {
		pr, err = api.PullRequestForBranch(apiClient, baseRepo, "", branchWithOwner)
		if err != nil {
			return fmt.Errorf("could not find pull request: %w", err)
		}
		prNum = pr.Number
	}

	out := colorableOut(cmd)

	if reviewData == nil {
		reviewData, err = reviewSurvey(cmd)
		if err != nil {
			return err
		}
		if reviewData == nil && err == nil {
			fmt.Fprint(out, "Discarding.\n")
			return nil
		}
	}

	err = api.AddReview(apiClient, pr, reviewData)
	if err != nil {
		return fmt.Errorf("failed to create review: %w", err)
	}

	switch reviewData.State {
	case api.ReviewComment:
		fmt.Fprintf(out, "%s Reviewed pull request #%d\n", utils.Gray("-"), prNum)
	case api.ReviewApprove:
		fmt.Fprintf(out, "%s Approved pull request #%d\n", utils.Green("✓"), prNum)
	case api.ReviewRequestChanges:
		fmt.Fprintf(out, "%s Requested changes to pull request #%d\n", utils.Red("+"), prNum)
	}

	return nil
}

func reviewSurvey(cmd *cobra.Command) (*api.PullRequestReviewInput, error) {
	editorCommand, err := determineEditor(cmd)
	if err != nil {
		return nil, err
	}

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

	err = SurveyAsk(typeQs, &typeAnswers)
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
		&survey.Question{
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

	err = SurveyAsk(bodyQs, &bodyAnswers)
	if err != nil {
		return nil, err
	}

	if bodyAnswers.Body == "" && (reviewState == api.ReviewComment || reviewState == api.ReviewRequestChanges) {
		return nil, errors.New("this type of review cannot be blank")
	}

	if len(bodyAnswers.Body) > 0 {
		out := colorableOut(cmd)
		renderedBody, err := utils.RenderMarkdown(bodyAnswers.Body)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(out, "Got:\n%s", renderedBody)
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

	err = SurveyAsk(confirmQs, &confirm)
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
