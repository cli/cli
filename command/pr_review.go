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
	// TODO re-register post release
	// prCmd.AddCommand(prReviewCmd)

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

	if found == 0 {
		return nil, nil // signal interactive mode
	} else if found > 1 {
		return nil, errors.New("need exactly one of --approve, --request-changes, or --comment")
	}

	body, err := cmd.Flags().GetString("body")
	if err != nil {
		return nil, err
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
	baseRepo, err := determineBaseRepo(cmd, ctx)
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
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

	input, err := processReviewOpt(cmd)
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
	}

	if input == nil {
		input, err = reviewSurvey(cmd)
		if err != nil {
			return err
		}
		if input == nil && err == nil {
			// Cancelled.
			return nil
		}
	}

	err = api.AddReview(apiClient, pr, input)
	if err != nil {
		return fmt.Errorf("failed to create review: %w", err)
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
					"Cancel",
				},
			},
		},
	}

	err = SurveyAsk(typeQs, &typeAnswers)
	if err != nil {
		return nil, err
	}

	reviewState := api.ReviewComment

	switch typeAnswers.ReviewType {
	case "Approve":
		reviewState = api.ReviewApprove
	case "Request Changes":
		reviewState = api.ReviewRequestChanges
	case "Cancel":
		return nil, nil
	}

	bodyAnswers := struct {
		Body string
	}{}

	bodyQs := []*survey.Question{
		&survey.Question{
			Name: "body",
			Prompt: &surveyext.GhEditor{
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
