package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/spf13/cobra"
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

	if found != 1 {
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

	err = api.AddReview(apiClient, pr, input)
	if err != nil {
		return fmt.Errorf("failed to create review: %w", err)
	}

	return nil
}
