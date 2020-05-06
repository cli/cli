package command

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
)

func prCheckout(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	currentBranch, _ := ctx.Branch()
	remotes, err := ctx.Remotes()
	if err != nil {
		return err
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	var baseRepo ghrepo.Interface
	prArg := args[0]
	if prNum, repo := prFromURL(prArg); repo != nil {
		prArg = prNum
		baseRepo = repo
	}

	if baseRepo == nil {
		baseRepo, err = determineBaseRepo(cmd, ctx)
		if err != nil {
			return err
		}
	}

	pr, err := prFromArg(apiClient, baseRepo, prArg)
	if err != nil {
		return err
	}

	baseRemote, _ := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())
	// baseRemoteSpec is a repository URL or a remote name to be used in git fetch
	baseURLOrName := formatRemoteURL(cmd, ghrepo.FullName(baseRepo))
	if baseRemote != nil {
		baseURLOrName = baseRemote.Name
	}

	headRemote := baseRemote
	if pr.IsCrossRepository {
		headRemote, _ = remotes.FindByRepo(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
	}

	localBranchName, err := cmd.Flags().GetString("branch")
	if err != nil {
		return err
	}
	if localBranchName == "" {
		localBranchName = fmt.Sprintf("pr/%d", pr.Number)
	}

	var cmdQueue [][]string
	if headRemote != nil {
		// there is an existing git remote for PR head
		srcRef := fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
		destRef := fmt.Sprintf("refs/remotes/%s/%s", headRemote.Name, pr.HeadRefName)
		refSpec := fmt.Sprintf("+%s:%s", srcRef, destRef)

		cmdQueue = append(cmdQueue, []string{"git", "fetch", headRemote.Name, refSpec})

		if _, err := git.ShowRefs("refs/heads/" + localBranchName); err == nil {
			// local branch already exists
			cmdQueue = append(cmdQueue, []string{"git", "checkout", localBranchName})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", destRef})
		} else {
			cmdQueue = append(cmdQueue, []string{"git", "checkout", "-b", localBranchName, "--no-track", destRef})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", localBranchName), headRemote.Name})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", localBranchName), srcRef})
		}
	} else {
		// no git remote for PR head

		ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)
		if localBranchName == currentBranch {
			// PR head matches currently checked out branch
			cmdQueue = append(cmdQueue, []string{"git", "fetch", baseURLOrName, ref})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", "FETCH_HEAD"})
		} else {
			// create a new branch
			cmdQueue = append(cmdQueue, []string{"git", "fetch", baseURLOrName, fmt.Sprintf("%s:%s", ref, localBranchName)})
			cmdQueue = append(cmdQueue, []string{"git", "checkout", localBranchName})
		}

		remote := baseURLOrName
		mergeRef := ref
		if pr.MaintainerCanModify {
			remote = formatRemoteURL(cmd, fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name))
			mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
		}
		if mc, err := git.Config(fmt.Sprintf("branch.%s.merge", localBranchName)); err != nil || mc == "" {
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", localBranchName), remote})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", localBranchName), mergeRef})
		}
	}

	for _, args := range cmdQueue {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := run.PrepareCmd(cmd).Run(); err != nil {
			return err
		}
	}

	return nil
}

var prCheckoutCmd = &cobra.Command{
	Use:   "checkout {<number> | <url> | <branch>}",
	Short: "Check out a pull request in Git",
	Long: `Check out a pull request locally.

	With '--branch <branch>', checkout the pull request into a specific local branch.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("pull request number or url or branch required as an argument")
		}
		return nil
	},
	RunE: prCheckout,
}
