package command

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

/*
SCENARIOS TO TEST:
 A. never checked out PR
 A1. cloned my fork of parent, never added parent
 A2. cloned my fork of parent, parent added as upstream
 A3. cloned my fork of parent, parent not added, but upstream remote present (for some reason)
 A4. cloned parent as origin, my fork added as fork (pr create setup)
 A5. cloned parent as origin, my fork added as something else entirely
 A6. -R specified
 B. have checked out PR already
 B1. cloned my fork of parent, never added parent
 B2. cloned my fork of parent, parent added as upstream
 B3. cloned my fork of parent, parent not added, but upstream remote present (for some reason)
 B4. cloned parent as origin, my fork added as fork (pr create setup)
 B5. cloned parent as origin, my fork added as something else entirely
 B6. -R specified

 A1 fails. We're not noticing that origin is a fork of something else and looking for a PR there.
 A2 succeeds. We set the remote for the branch appropriately and find the PR in upstream.
 A3 "fails" but, like, in that it tries to use the weird upstream and can't find PR. this is a weird case anyway.
 A4 works
 A5 works
 A6 works, but interestingly fails in a different way than A1. It's the graphql could not find PR instead of 'not found.' clearly, error wrapping needs to be improved.
*/

func prCheckout(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	currentBranch, _ := ctx.Branch()
	configRemotes, err := ctx.Remotes()
	out := colorableOut(cmd)
	if err != nil {
		return fmt.Errorf("failed to read git config: %w", err)
	}
	resolvedRemotes, err := resolveRemotesForCommand(cmd, ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve local remotes to github repos: %w", err)
	}

	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to initiate API client: %w", err)
	}

	baseRepo, err := resolvedRemotes.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	fmt.Fprintf(out, "Querying %s for PR %s\n", utils.Cyan(ghrepo.FullName(baseRepo)), utils.Cyan(args[0]))

	var baseRemote *context.Remote
	baseRemote, err = resolvedRemotes.RemoteForRepo(baseRepo)
	remoteNotFound := err != nil
	if remoteNotFound {
		fmt.Fprintf(out, "Adding remote for %s at %s\n", utils.Cyan(ghrepo.FullName(baseRepo)), utils.Cyan("upstream"))
		// TODO handle ssh
		baseRepoURL := fmt.Sprintf("https://github.com/%s.git", ghrepo.FullName(baseRepo))
		gitRemote, err := git.AddRemote("upstream", baseRepoURL, "")
		if err != nil {
			return fmt.Errorf("error adding remote: %w", err)
		}
		baseRemote = &context.Remote{
			Remote: gitRemote,
			Owner:  baseRepo.RepoOwner(),
			Repo:   baseRepo.RepoName(),
		}
	}

	pr, err := prFromArg(apiClient, baseRepo, args[0])
	if err != nil {
		return fmt.Errorf("failed to fetch pull request: %w", err)
	}

	// baseRemote: where the PR lives
	// headRemote: where the branch (headRefName) attached to the PR lives

	// We have a PR and a baseRepo. Need to determine where the PR's branch lives as it might not live
	// in baseRepo (where the PR resides).
	headRemote := baseRemote
	if pr.IsCrossRepository {
		headRemote, err = configRemotes.FindByRepo(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
		headRemoteNotFound := err != nil
		if headRemoteNotFound {
			// The pr originates from a repo we don't have in remotes yet. Let's add one.
			headRepoOwner := pr.HeadRepositoryOwner.Login
			headRepoName := pr.HeadRepository.Name
			headRepoFullName := fmt.Sprintf("%s/%s", headRepoOwner, headRepoName)
			headRepoURL := fmt.Sprintf("https://github.com/%s.git", headRepoFullName)
			fmt.Fprintf(out, "Adding remote for %s at %s\n", utils.Cyan(headRepoFullName), utils.Cyan(headRepoOwner))
			gitRemote, err := git.AddRemote(headRepoOwner, headRepoURL, "")
			if err != nil {
				return fmt.Errorf("error adding remote: %w", err)
			}
			headRemote = &context.Remote{
				Remote: gitRemote,
				Owner:  headRepoOwner,
				Repo:   headRepoName,
			}
		}
	}

	// TODO delete
	fmt.Printf("BASE REMOTE: %s\n", ghrepo.FullName(baseRemote))
	fmt.Printf("HEAD REMOTE: %s\n", ghrepo.FullName(headRemote))

	cmdQueue := [][]string{}

	newBranchName := pr.HeadRefName

	// Things to do:
	// 1. detect and handle when newBranchName exists but is for a different branch
	// 2. detect and handle when newBranchName is the same as default branch name
	// 3. detect and handle when pr can be modified by maintainer (POSSIBLY NO-OP)

	// TODO why is this checking head repo? aren't we concerned about base repo's default branch?
	// additionally, won't we catch this if we just dedupe?
	if newBranchName == pr.HeadRepository.DefaultBranchRef.Name {
		// TODO warn the user that we did this in case they want to git push
		newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
	}
	branchExists := git.VerifyRef("refs/heads/" + newBranchName)
	if branchExists {
		// TODO figure out how to tell if the existing branch /is/ a prior checkout of the PR's branch
		// vs. an unrelated branch of the same name.
	}

	// BUG we are not negotiating default branch detetction when cross repo and named remote
	if headRemote != nil {
		// there is an existing git remote for PR head
		// TODO refSpec use is confusing here; what is wrong with just a `git fetch name` and letting
		// git decide the refspec?
		remoteBranch := fmt.Sprintf("%s/%s", headRemote.Name, pr.HeadRefName)
		refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pr.HeadRefName, remoteBranch)

		cmdQueue = append(cmdQueue, []string{"git", "fetch", headRemote.Name, refSpec})

		// local branch already exists
		if git.VerifyRef("refs/heads/" + newBranchName) {
			// TODO dedupe branch name and warn user potentially
			cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
			// TERMINUS we fetched a named remote, switched branches, merged local branch with remote
			// branch
		} else {
			// TODO THIS IS NOW DEAD CODE
			// TODO why not let git write the config values with --track?
			cmdQueue = append(cmdQueue, []string{"git", "checkout", "-b", newBranchName, "--no-track", remoteBranch})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), headRemote.Name})
			cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), "refs/heads/" + pr.HeadRefName})
		}
	} else {
		// no git remote for PR head

		// avoid naming the new branch the same as the default branch
		if newBranchName == pr.HeadRepository.DefaultBranchRef.Name {
			// TODO warn the user that we did this in case they want to git push
			newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
		}

		ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)
		if newBranchName == currentBranch {
			// PR head matches currently checked out branch
			cmdQueue = append(cmdQueue, []string{"git", "fetch", baseRemote.Name, ref})
			cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", "FETCH_HEAD"})
		} else {
			// create a new branch
			// TODO again with the refspecs?
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
