package command

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/utils"
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
	color, err := cmd.Flags().GetString("color")
	if err != nil {
		return err
	}
	if !validColorFlag(color) {
		return fmt.Errorf("did not understand color: %q. Expected one of always, never, or auto", color)
	}

	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	baseRepo, err := determineBaseRepo(apiClient, cmd, ctx)
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	// begin pr resolution boilerplate
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

	if prNum < 1 {
		pr, err := api.PullRequestForBranch(apiClient, baseRepo, "", branchWithOwner)
		if err != nil {
			return fmt.Errorf("could not find pull request: %w", err)
		}
		prNum = pr.Number
	}
	// end pr resolution boilerplate

	diff, err := apiClient.PullRequestDiff(baseRepo, prNum)
	if err != nil {
		return fmt.Errorf("could not find pull request diff: %w", err)
	}

	out := cmd.OutOrStdout()
	if color == "auto" {
		color = "never"
		isTTY := false
		if outFile, isFile := out.(*os.File); isFile {
			isTTY = utils.IsTerminal(outFile)
			if isTTY {
				color = "always"
			}
		}
	}

	if color == "never" {
		fmt.Fprint(out, diff)
		return nil
	}

	out = colorableOut(cmd)
	for _, diffLine := range strings.Split(diff, "\n") {
		output := diffLine
		switch {
		case isHeaderLine(diffLine):
			output = utils.Bold(diffLine)
		case isAdditionLine(diffLine):
			output = utils.Green(diffLine)
		case isRemovalLine(diffLine):
			output = utils.Red(diffLine)
		}

		fmt.Fprintln(out, output)
	}

	return nil
}

func isHeaderLine(dl string) bool {
	prefixes := []string{"+++", "---", "diff", "index"}
	for _, p := range prefixes {
		if strings.HasPrefix(dl, p) {
			return true
		}
	}
	return false
}

func isAdditionLine(dl string) bool {
	return strings.HasPrefix(dl, "+")
}

func isRemovalLine(dl string) bool {
	return strings.HasPrefix(dl, "-")
}

func validColorFlag(c string) bool {
	return c == "auto" || c == "always" || c == "never"
}
