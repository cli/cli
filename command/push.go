package command

import (
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/utils"
	"github.com/spf13/cobra"
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
	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond, spinner.WithSuffix(" transferring your commits to GitHub."))
	s.Start()
	err := git.Run("push")
	s.Stop()

	if err == nil {
		fmt.Printf("All commits transferred to GitHub.\n")
		return nil
	}

	hasPermission, apiErr := api.HasPushPermission()
	if apiErr != nil {
		return apiErr
	}

	// Fail because the user has permission to push but `git push` failed for some other reason
	if hasPermission {
		return err
	}

	err = fork()
	if err != nil {
		return err
	}
	return push(cmd, args)
}

func fork() error {
	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return err
	}

	b := fmt.Sprintf("[%s/%s]", ghRepo.Owner, ghRepo.Name)
	fmt.Printf("You don't have permission to push to %s\n", utils.Cyan(b))

	m := "Would you like to fork this repo?"
	var shouldFork bool
	err = survey.AskOne(&survey.Confirm{Message: m, Default: true}, &shouldFork)
	if err != nil {
		return err
	}

	if !shouldFork {
		return fmt.Errorf("failed because you don't have permission to push to %s/%s", ghRepo.Owner, ghRepo.Name)
	}

	url, cloneURL, upstreamCloneURL, err := api.Fork()
	if err != nil {
		return err
	}

	fmt.Printf("All your future changes will be pushed to your fork at %s.\n", url)

	err = git.Run("remote", "set-url", "origin", cloneURL)
	if err != nil {
		return err
	}

	upstreamName := "gh-cli-upstream"
	git.Run("remote", "add", upstreamName, upstreamCloneURL) // Ignore this error
	return nil
}
