package command

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
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
	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return err
	}

	err = git.Run("push")
	if err == nil {
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

	m := fmt.Sprintf("You don't have permission to push to %s/%s. Would you like to fork this repo?", ghRepo.Owner, ghRepo.Name)

	var shouldFork bool
	err = survey.AskOne(&survey.Confirm{Message: m, Default: true}, &shouldFork)
	if err != nil {
		return err
	}

	if !shouldFork {
		return fmt.Errorf("failed because you don't have permission to push to %s/%s", ghRepo.Owner, ghRepo.Name)
	}

	m = fmt.Sprintf("Where should we fork desktop?")
	o, err := api.Orgs()
	if err != nil {
		return err
	}

	var org string
	err = survey.AskOne(&survey.Select{Message: m, Options: o}, &org)
	if err != nil {
		return err
	}

	fmt.Printf("🌭 YOU ARE ABOUT TO FORK THIS TO %+v\n", org)

	return nil
}
