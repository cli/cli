package command

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/cli/cli/git"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func prCheckout(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	currentBranch, _ := ctx.Branch()
	remotes, err := ctx.Remotes()
	if err != nil {
		return err
	}
	// FIXME: duplicates logic from fsContext.BaseRepo
	baseRemote, err := remotes.FindByName("upstream", "github", "origin", "*")
	if err != nil {
		return err
	}
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	pr, err := prFromArg(apiClient, baseRemote, args[0])
	if err != nil {
		return err
	}

	headRemote := baseRemote
	if pr.IsCrossRepository {
		headRemote, _ = remotes.FindByRepo(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
	}

	cmdQueue := make([][]string, 0, 4)
	newBranchName := pr.HeadRefName
	if headRemote != nil {
		// there is an existing git remote for PR head
		remoteBranch := fmt.Sprintf("%s/%s", headRemote.Name, pr.HeadRefName)
		refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pr.HeadRefName, remoteBranch)

		cmdQueue = append(cmdQueue, []string{"git", "fetch", headRemote.Name, refSpec})

		// local branch already exists
		if git.VerifyRef("refs/heads/" + newBranchName) {
			cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
		} else {
			cmdQueue = append(cmdQueue, []string{"git", "checkout", "-b", newBranchName, "--no-track", remoteBranch})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), headRemote.Name})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), "refs/heads/" + pr.HeadRefName})
		}
	} else {
		// no git remote for PR head

		// avoid naming the new branch the same as the default branch
		if newBranchName == pr.HeadRepository.DefaultBranchRef.Name {
			newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
		}

		ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)
		if newBranchName == currentBranch {
			// PR head matches currently checked out branch
			cmdQueue = append(cmdQueue, []string{"git", "fetch", baseRemote.Name, ref})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", "FETCH_HEAD"})
		} else {
			// create a new branch
			cmdQueue = append(cmdQueue, []string{"git", "fetch", baseRemote.Name, fmt.Sprintf("%s:%s", ref, newBranchName)})
			cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
		}

		remote := baseRemote.Name
		mergeRef := ref
		if pr.MaintainerCanModify {
			remote = fmt.Sprintf("https://github.com/%s/%s.git", pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
			mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
		}
		if mc, err := git.Config(fmt.Sprintf("branch.%s.merge", newBranchName)); err != nil || mc == "" {
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), remote})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), mergeRef})
		}
	}

	for _, args := range cmdQueue {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := utils.PrepareCmd(cmd).Run(); err != nil {
			return err
		}
	}

	return nil
}

var prCheckoutCmd = &cobra.Command{
	Use:   "checkout {<number> | <url> | <branch>}",
	Short: "Check out a pull request in Git",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("requires a PR number as an argument")
		}
		return nil
	},
	RunE: prCheckout,
}
