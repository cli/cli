package checkout

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/safeexec"
	"github.com/spf13/cobra"
)

type CheckoutOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	Finder shared.PRFinder

	SelectorArg       string
	RecurseSubmodules bool
	Force             bool
	Detach            bool
}

func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "checkout {<number> | <url> | <branch>}",
		Short: "Check out a pull request in git",
		Args:  cmdutil.ExactArgs(1, "argument required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.RecurseSubmodules, "recurse-submodules", "", false, "Update all submodules after checkout")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Reset the existing local branch to the latest state of the pull request")
	cmd.Flags().BoolVarP(&opts.Detach, "detach", "", false, "Checkout PR with a detached HEAD")

	return cmd
}

func checkoutRun(opts *CheckoutOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocol, _ := cfg.Get(baseRepo.RepoHost(), "git_protocol")

	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}
	baseRemote, _ := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())
	baseURLOrName := ghrepo.FormatRemoteURL(baseRepo, protocol)
	if baseRemote != nil {
		baseURLOrName = baseRemote.Name
	}

	headRemote := baseRemote
	if pr.IsCrossRepository {
		headRemote, _ = remotes.FindByRepo(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name)
	}

	if strings.HasPrefix(pr.HeadRefName, "-") {
		return fmt.Errorf("invalid branch name: %q", pr.HeadRefName)
	}

	var cmdQueue [][]string

	if headRemote != nil {
		cmdQueue = append(cmdQueue, cmdsForExistingRemote(headRemote, pr, opts)...)
	} else {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		apiClient := api.NewClientFromHTTP(httpClient)

		defaultBranch, err := api.RepoDefaultBranch(apiClient, baseRepo)
		if err != nil {
			return err
		}
		cmdQueue = append(cmdQueue, cmdsForMissingRemote(pr, baseURLOrName, baseRepo.RepoHost(), defaultBranch, protocol, opts)...)
	}

	if opts.RecurseSubmodules {
		cmdQueue = append(cmdQueue, []string{"git", "submodule", "sync", "--recursive"})
		cmdQueue = append(cmdQueue, []string{"git", "submodule", "update", "--init", "--recursive"})
	}

	err = executeCmds(cmdQueue)
	if err != nil {
		return err
	}

	return nil
}

func cmdsForExistingRemote(remote *context.Remote, pr *api.PullRequest, opts *CheckoutOptions) [][]string {
	var cmds [][]string

	remoteBranch := fmt.Sprintf("%s/%s", remote.Name, pr.HeadRefName)

	refSpec := fmt.Sprintf("+refs/heads/%s", pr.HeadRefName)
	if !opts.Detach {
		refSpec += fmt.Sprintf(":refs/remotes/%s", remoteBranch)
	}

	cmds = append(cmds, []string{"git", "fetch", remote.Name, refSpec})

	switch {
	case opts.Detach:
		cmds = append(cmds, []string{"git", "checkout", "--detach", "FETCH_HEAD"})
	case localBranchExists(pr.HeadRefName):
		cmds = append(cmds, []string{"git", "checkout", pr.HeadRefName})
		if opts.Force {
			cmds = append(cmds, []string{"git", "reset", "--hard", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
		} else {
			// TODO: check if non-fast-forward and suggest to use `--force`
			cmds = append(cmds, []string{"git", "merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
		}
	default:
		cmds = append(cmds, []string{"git", "checkout", "-b", pr.HeadRefName, "--no-track", remoteBranch})
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.remote", pr.HeadRefName), remote.Name})
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.merge", pr.HeadRefName), "refs/heads/" + pr.HeadRefName})
	}

	return cmds
}

func cmdsForMissingRemote(pr *api.PullRequest, baseURLOrName, repoHost, defaultBranch, protocol string, opts *CheckoutOptions) [][]string {
	var cmds [][]string

	newBranchName := pr.HeadRefName
	// avoid naming the new branch the same as the default branch
	if newBranchName == defaultBranch {
		newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
	}

	ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)

	if opts.Detach {
		cmds = append(cmds, []string{"git", "fetch", baseURLOrName, ref})
		cmds = append(cmds, []string{"git", "checkout", "--detach", "FETCH_HEAD"})
		return cmds
	}

	currentBranch, _ := opts.Branch()
	if newBranchName == currentBranch {
		// PR head matches currently checked out branch
		cmds = append(cmds, []string{"git", "fetch", baseURLOrName, ref})
		if opts.Force {
			cmds = append(cmds, []string{"git", "reset", "--hard", "FETCH_HEAD"})
		} else {
			// TODO: check if non-fast-forward and suggest to use `--force`
			cmds = append(cmds, []string{"git", "merge", "--ff-only", "FETCH_HEAD"})
		}
	} else {
		// create a new branch
		if opts.Force {
			cmds = append(cmds, []string{"git", "fetch", baseURLOrName, fmt.Sprintf("%s:%s", ref, newBranchName), "--force"})
		} else {
			// TODO: check if non-fast-forward and suggest to use `--force`
			cmds = append(cmds, []string{"git", "fetch", baseURLOrName, fmt.Sprintf("%s:%s", ref, newBranchName)})
		}
		cmds = append(cmds, []string{"git", "checkout", newBranchName})
	}

	remote := baseURLOrName
	mergeRef := ref
	if pr.MaintainerCanModify {
		headRepo := ghrepo.NewWithHost(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name, repoHost)
		remote = ghrepo.FormatRemoteURL(headRepo, protocol)
		mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
	}
	if missingMergeConfigForBranch(newBranchName) {
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), remote})
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), mergeRef})
	}

	return cmds
}

func missingMergeConfigForBranch(b string) bool {
	mc, err := git.Config(fmt.Sprintf("branch.%s.merge", b))
	return err != nil || mc == ""
}

func localBranchExists(b string) bool {
	_, err := git.ShowRefs("refs/heads/" + b)
	return err == nil
}

func executeCmds(cmdQueue [][]string) error {
	for _, args := range cmdQueue {
		// TODO: reuse the result of this lookup across loop iteration
		exe, err := safeexec.LookPath(args[0])
		if err != nil {
			return err
		}
		cmd := exec.Command(exe, args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := run.PrepareCmd(cmd).Run(); err != nil {
			return err
		}
	}
	return nil
}
