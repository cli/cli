package command

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"os/exec"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/utils"
)

func init() {
	RootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of local work and current pull request.",
	Long: `This command augments git status with additional 
context on any current PR open on GitHub for the current branch.`,
	RunE: status,
}

func status(cmd *cobra.Command, args []string) error {
	gitCmd := exec.Command("git", os.Args[1:]...)
	gitCmd.Stdout = os.Stdout

	err := gitCmd.Run()

	if err != nil {
		return errors.Wrap(err, "git failed")
	}

	fmt.Println()

	ctx := contextForCommand(cmd)
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return errors.Wrap(err, "couldn't create api client")
	}

	baseRepo, err := ctx.BaseRepo()
	if err != nil {
		return errors.Wrap(err, "couldn't determine current repository")
	}
	currentBranch, err := ctx.Branch()
	if err != nil {
		return errors.Wrap(err, "couldn't determine current branch")
	}
	currentUser, err := ctx.AuthLogin()
	if err != nil {
		return errors.Wrap(err, "couldn't determine current user")
	}

	prPayload, err := api.PullRequests(apiClient, baseRepo, currentBranch, currentUser)
	if err != nil {
		return errors.Wrap(err, "api call failed")
	}

	if prPayload.CurrentPR != nil {
		pr := prPayload.CurrentPR
		printHeader("Current PR")
		fmt.Printf("#%d %s %s", pr.Number, truncate(50, pr.Title), utils.Cyan("["+pr.HeadRefName+"]"))
		if checks := pr.ChecksStatus(); checks.Total > 0 {
			ratio := fmt.Sprintf("%d/%d", checks.Passing, checks.Total)
			if checks.Failing > 0 {
				ratio = utils.Red(ratio)
			} else if checks.Pending > 0 {
				ratio = utils.Yellow(ratio)
			} else if checks.Passing == checks.Total {
				ratio = utils.Green(ratio)
			}
			fmt.Println()
			fmt.Printf(" - checks: %s", ratio)
		}
		reviews := pr.ReviewStatus()
		if reviews.ChangesRequested {
			fmt.Println()
			fmt.Printf(" - %s", utils.Red("changes requested"))
		} else if reviews.Approved {
			fmt.Println()
			fmt.Printf(" - %s", utils.Green("approved"))
		}
		fmt.Println()
	} else {
		message := fmt.Sprintf("There is no pull request associated with %s", utils.Cyan("["+currentBranch+"]"))
		printMessage(message)
	}

	return nil
}
