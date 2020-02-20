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

	/*
		  High level notes about this code:

			- We intentionally do not add any remotes during a checkout. In general adding remotes can
			  lead to confusion (ie adding an upstream or parent remote) or very long lists of remotes (ie
				adding a remote for every fork that opens a PR on a prent). Thus, we use remote URLs and
				refspecs explicitly instead of named remotes + tracking.
			- We prefer explicit invocations of git; for example, specifying refspecs on both the remote
			  and local ends when doing a fetch. The less we rely on git's implicit behavior around
			  fetching/pulling/merging the more stable our code is over time as git's behavior changes.
	*/

	headRemote := baseRemote
	if pr.IsCrossRepository {
		headRemote, _ = remotes.FindByRepo(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
	}

	cmdQueue := [][]string{}

	newBranchName := pr.HeadRefName
	if headRemote != nil {
		// there is an existing git remote for PR head
		// note that this conditional does double duty: since we initialize headRemote to baseRemote,
		// being in this conditional may also mean that a given PR is not cross-repo and base/head are
		// the same. Given that we read baseRemote from the git config (and error if we can't find
		// something suitable) we're guaranteed that headRemote is not nil in that case.
		remoteBranch := fmt.Sprintf("%s/%s", headRemote.Name, pr.HeadRefName)
		refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pr.HeadRefName, remoteBranch)

		cmdQueue = append(cmdQueue, []string{"git", "fetch", headRemote.Name, refSpec})

		// local branch already exists
		if git.VerifyRef("refs/heads/" + newBranchName) {
			// TODO #233 verify that this ref is actually related to the PR's headRef
			cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
		} else {
			// we need a new branch locally to hold the PR's branch
			cmdQueue = append(cmdQueue, []string{"git", "checkout", "-b", newBranchName, "--no-track", remoteBranch})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), headRemote.Name})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), "refs/heads/" + pr.HeadRefName})
		}
	} else {
		// TODO talk to mislav: we end up here if the PR *IS* cross repo and there *IS* a remote named
		// for the headRepo. We set the new branch's remote to the base repo which I think is incorrect
		// no git remote for PR head. we branch like this because given our constraint above about not
		// adding new remotes we run git in different ways (remote URLs + explicit refspecs)

		// avoid naming the new branch the same as the default branch
		// TODO for cross-repo PRs this should be checking the default branch on the base repo.
		if newBranchName == pr.HeadRepository.DefaultBranchRef.Name {
			newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
		}

		// We default to using this GitHub-managed ref that stores a PR branch. It lives in the
		// baseRemote and cannot be pushed to. If we determine that pr.MaintainerCanModify is true,
		// we'll reconfigure to use a ref we can push to.
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
		// potentially use a different ref on the base remote. this allows us to actually push commits
		// to the PR.
		if pr.MaintainerCanModify {
			remote = fmt.Sprintf("https://github.com/%s/%s.git", pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
			mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
		}

		// Configure remote/merge settings
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
