package checkout

import (
	"errors"
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
	"github.com/spf13/cobra"
)

type CheckoutOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	SelectorArg       string
	RecurseSubmodules bool
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
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &cmdutil.FlagError{Err: errors.New("argument required")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.RecurseSubmodules, "recurse-submodules", "", false, "Update all active submodules (recursively)")

	return cmd
}

func checkoutRun(opts *CheckoutOptions) error {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, baseRepo, err := shared.PRFromArgs(apiClient, opts.BaseRepo, opts.Branch, opts.Remotes, opts.SelectorArg)
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	protocol, _ := cfg.Get(baseRepo.RepoHost(), "git_protocol")

	baseRemote, _ := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())

	// baseURLOrName is a repository URL or a remote name to be used in git fetch
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

	if headRemote == nil {
		// no git remote for PR head
		currentBranch, _ := opts.Branch()

		defaultBranchName, err := api.RepoDefaultBranch(apiClient, baseRepo)
		if err != nil {
			return err
		}
		cmdQueue = append(cmdQueue, checkoutFromMissingRemoteCmds(baseURLOrName, pr, baseRepo.RepoHost(), defaultBranchName, currentBranch, protocol)...)
	} else {
		cmdQueue = append(cmdQueue, checkoutFromExistingRemoteCmds(headRemote, pr)...)
	}

	if opts.RecurseSubmodules {
		cmdQueue = append(cmdQueue, recurseThroughSubmodulesCmds()...)
	}

	err = executeCmdQueue(cmdQueue)
	if err != nil {
		return err
	}

	return nil
}

func recurseThroughSubmodulesCmds() [][]string {
	return [][]string{
		{"git", "submodule", "sync", "--recursive"},
		{"git", "submodule", "update", "--init", "--recursive"},
	}
}

func checkoutFromExistingRemoteCmds(remote *context.Remote, pr *api.PullRequest) [][]string {
	var cmds [][]string

	remoteBranch := fmt.Sprintf("%s/%s", remote.Name, pr.HeadRefName)
	refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pr.HeadRefName, remoteBranch)

	cmds = append(cmds, []string{"git", "fetch", remote.Name, refSpec})

	// local branch already exists
	if _, err := git.ShowRefs("refs/heads/" + pr.HeadRefName); err == nil {
		cmds = append(cmds, []string{"git", "checkout", pr.HeadRefName})
		cmds = append(cmds, []string{"git", "merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
	} else {
		cmds = append(cmds, []string{"git", "checkout", "-b", pr.HeadRefName, "--no-track", remoteBranch})
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.remote", pr.HeadRefName), remote.Name})
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.merge", pr.HeadRefName), "refs/heads/" + pr.HeadRefName})
	}

	return cmds
}

func checkoutFromMissingRemoteCmds(baseURLOrName string, pr *api.PullRequest, repoHost string, defaultBranch string, currentBranch string, protocol string) [][]string {
	var cmds [][]string

	newBranchName := pr.HeadRefName
	// avoid naming the new branch the same as the default branch
	if newBranchName == defaultBranch {
		newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
	}

	ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)
	if newBranchName == currentBranch {
		// PR head matches currently checked out branch
		cmds = append(cmds, []string{"git", "fetch", baseURLOrName, ref})
		cmds = append(cmds, []string{"git", "merge", "--ff-only", "FETCH_HEAD"})
	} else {
		// create a new branch
		cmds = append(cmds, []string{"git", "fetch", baseURLOrName, fmt.Sprintf("%s:%s", ref, newBranchName)})
		cmds = append(cmds, []string{"git", "checkout", newBranchName})
	}

	remote := baseURLOrName
	mergeRef := ref
	if pr.MaintainerCanModify {
		headRepo := ghrepo.NewWithHost(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name, repoHost)
		remote = ghrepo.FormatRemoteURL(headRepo, protocol)
		mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
	}

	if mc, err := git.Config(fmt.Sprintf("branch.%s.merge", newBranchName)); err != nil || mc == "" {
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), remote})
		cmds = append(cmds, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), mergeRef})
	}

	return cmds
}

func executeCmdQueue(q [][]string) error {
	for _, args := range q {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := run.PrepareCmd(cmd).Run(); err != nil {
			return err
		}
	}
	return nil
}
