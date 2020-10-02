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
	DetachHead        bool
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
	cmd.Flags().BoolVarP(&opts.DetachHead, "detach-head", "", false, "Checkout PR with a detach head")

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

	if headRemote != nil {
		cmdQueue = append(cmdQueue, fetchFromExistingRemote(headRemote, pr)...)
	} else {
		// no git remote for PR head
		currentBranch, _ := opts.Branch()

		defaultBranchName, err := api.RepoDefaultBranch(apiClient, baseRepo)
		if err != nil {
			return err
		}
		cmdQueue = append(cmdQueue, fetchFromNewRemote(protocol, apiClient, pr, baseRepo, defaultBranchName, currentBranch, baseURLOrName)...)
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
	var cmdQueue [][]string
	cmdQueue = append(cmdQueue, []string{"git", "submodule", "sync", "--recursive"})
	cmdQueue = append(cmdQueue, []string{"git", "submodule", "update", "--init", "--recursive"})
	return cmdQueue
}

func fetchFromExistingRemote(headRemote *context.Remote, pr *api.PullRequest) [][]string {
	var cmdQueue [][]string

	remoteBranch := fmt.Sprintf("%s/%s", headRemote.Name, pr.HeadRefName)
	refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s", pr.HeadRefName, remoteBranch)
	newBranchName := pr.HeadRefName

	cmdQueue = append(cmdQueue, []string{"git", "fetch", headRemote.Name, refSpec})

	// local branch already exists
	if _, err := git.ShowRefs("refs/heads/" + newBranchName); err == nil {
		cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
		cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", fmt.Sprintf("refs/remotes/%s", remoteBranch)})
	} else {
		cmdQueue = append(cmdQueue, []string{"git", "checkout", "-b", newBranchName, "--no-track", remoteBranch})
		cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), headRemote.Name})
		cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), "refs/heads/" + pr.HeadRefName})
	}

	return cmdQueue
}

func fetchFromNewRemote(protocol string, apiClient *api.Client, pr *api.PullRequest, baseRepo ghrepo.Interface, defaultBranchName string, currentBranch string, baseURLOrName string) [][]string {
	var cmdQueue [][]string

	newBranchName := pr.HeadRefName

	// avoid naming the new branch the same as the default branch
	if newBranchName == defaultBranchName {
		newBranchName = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, newBranchName)
	}

	ref := fmt.Sprintf("refs/pull/%d/head", pr.Number)
	if newBranchName == currentBranch {
		// PR head matches currently checked out branch
		cmdQueue = append(cmdQueue, []string{"git", "fetch", baseURLOrName, ref})
		cmdQueue = append(cmdQueue, []string{"git", "merge", "--ff-only", "FETCH_HEAD"})
	} else {
		// create a new branch
		cmdQueue = append(cmdQueue, []string{"git", "fetch", baseURLOrName, fmt.Sprintf("%s:%s", ref, newBranchName)})
		cmdQueue = append(cmdQueue, []string{"git", "checkout", newBranchName})
	}

	remote := baseURLOrName
	mergeRef := ref
	if pr.MaintainerCanModify {
		headRepo := ghrepo.NewWithHost(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name, baseRepo.RepoHost())
		remote = ghrepo.FormatRemoteURL(headRepo, protocol)
		mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
	}
	if mc, err := git.Config(fmt.Sprintf("branch.%s.merge", newBranchName)); err != nil || mc == "" {
		cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.remote", newBranchName), remote})
		cmdQueue = append(cmdQueue, []string{"git", "config", fmt.Sprintf("branch.%s.merge", newBranchName), mergeRef})
	}

	return cmdQueue
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
