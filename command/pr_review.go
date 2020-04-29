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
	prCmd.AddCommand(prReviewCmd)

	prReviewCmd.Flags().StringP("approve", "a", "", "Approve pull request")
	prReviewCmd.Flags().StringP("request-changes", "r", "", "Request changes on a pull request")
	prReviewCmd.Flags().StringP("comment", "c", "", "Comment on a pull request")

	// this is required; without it pflag complains that you must pass a string to string flags.
	prReviewCmd.Flags().Lookup("approve").NoOptDefVal = " "
	prReviewCmd.Flags().Lookup("request-changes").NoOptDefVal = " "
	prReviewCmd.Flags().Lookup("comment").NoOptDefVal = " "
}

var prReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "TODO",
	Args:  cobra.MaximumNArgs(1),
	Long:  "TODO",
	RunE:  prReview,
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
		return nil, errors.New("need exactly one of approve, request-changes, or comment")
	}

	val, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}

	body := ""
	if val != " " {
		body = val
	}

	if flag == "comment" && (body == " " || len(body) == 0) {
		return nil, errors.New("cannot leave blank comment")
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

	if len(args) == 0 {
		prNum, _, err = prSelectorForCurrentBranch(ctx, baseRepo)
		if err != nil {
			return fmt.Errorf("could not query for pull request for current branch: %w", err)
		}
	} else {
		prArg, repo := prFromURL(args[0])
		if repo != nil {
			baseRepo = repo
			// TODO handle malformed URL; it falls through to Atoi
		} else {
			prArg = strings.TrimPrefix(args[0], "#")
		}
		prNum, err = strconv.Atoi(prArg)
		if err != nil {
			return fmt.Errorf("could not parse pull request number: %w", err)
		}
	}

	input, err := processReviewOpt(cmd)
	if err != nil {
		return fmt.Errorf("did not understand desired review action: %w", err)
	}

	pr, err := api.PullRequestByNumber(apiClient, baseRepo, prNum)
	if err != nil {
		return fmt.Errorf("could not find pull request: %w", err)
	}

	err = api.AddReview(apiClient, pr, input)
	if err != nil {
		return fmt.Errorf("failed to create review: %w", err)
	}

	return nil
}
