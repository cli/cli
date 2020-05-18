package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/spf13/cobra"
)

var prDiffCmd = &cobra.Command{
	Use:   "diff {<number> | <url>}",
	Short: "View a pull request's changes.",
	RunE:  prDiff,
}

func init() {
	prDiffCmd.Flags().StringP("color", "c", "auto", "Whether or not to output color: {always|never|auto}")

	prCmd.AddCommand(prDiffCmd)
}

func prDiff(cmd *cobra.Command, args []string) error {
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

	fmt.Println(pr.Title)

	return nil
}
