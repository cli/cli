package command

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os/exec"
	"regexp"

	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/utils"
)

var (
	ssbRe = regexp.MustCompile("^## [^ ]+$")
)

func init() {
	var cmd = &cobra.Command{
		Use:   "push",
		Short: "Push commits",
		Long:  `Transfers commits you made on your computer to GitHub.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return push(cmd, args)
		},
	}

	RootCmd.AddCommand(cmd)
}

func push(cmd *cobra.Command, args []string) error {

	ctx := contextForCommand(cmd)
	currentBranch, err := ctx.Branch()
	if err != nil {
		return errors.Wrap(err, "couldn't determine current branch")
	}

	fmt.Println(
		utils.Bold("Transferring commits on"),
		utils.Cyan(currentBranch),
		utils.Bold("to GitHub..."))

	statusCmd := exec.Command("git", "status", "-sb")
	output, err := statusCmd.Output()
	if err != nil {
		errors.Wrap(err, "git failed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Scan()
	ssb := scanner.Text()

	ssbRe := regexp.MustCompile(fmt.Sprintf("^## %s\\.\\.\\.(.+?)/(.+)$", currentBranch))

	match := ssbRe.FindStringSubmatch(ssb)

	if len(match) == 0 {
		remotes, err := ctx.Remotes()
		if err != nil {
			return errors.Wrap(err, "couldn't determine remotes")
		}

		if len(remotes) == 0 {
			return fmt.Errorf("repo has no remotes")
		}

		remote := remotes[0]

		if len(remotes) > 1 {
			remoteOptions := []string{}
			for _, r := range remotes {
				remoteOptions = append(remoteOptions,
					fmt.Sprintf("%s/%s", r.Owner, r.Repo))
			}
			trackingSelectAnswer := struct {
				Selection int
			}{}
			trackingQs := []*survey.Question{
				{
					Name: "selection",
					Prompt: &survey.Select{
						Message: "Where would you like to push this branch to?",
						Options: remoteOptions,
					},
				},
			}
			err = survey.Ask(trackingQs, &trackingSelectAnswer)
			if err != nil {
				return errors.Wrap(err, "failed to prompt")
			}
			remote = remotes[trackingSelectAnswer.Selection]
		}

		err = setTrackingBranch(remote, currentBranch)
		if err != nil {
			return errors.Wrap(err, "couldn't set tracking branch")
		}
		fmt.Println(utils.Yellow("Now tracking at " + fmt.Sprintf("%s/%s (%s/%s)", remote.Owner, remote.Repo, remote.Name, currentBranch)))
	}

	err = git.Run("push")
	if err != nil {
		return errors.Wrap(err, "git push failed")
	}
	fmt.Println("Done.")

	return nil
}

func setTrackingBranch(remote *context.Remote, branch string) error {
	return git.Run("branch",
		fmt.Sprintf("--set-upstream-to=%s/%s", remote.Name, branch),
		branch)
}
