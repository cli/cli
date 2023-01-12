package checkout

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	cliContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CheckoutOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Remotes    func() (cliContext.Remotes, error)
	Branch     func() (string, error)

	Finder shared.PRFinder

	SelectorArg       string
	RecurseSubmodules bool
	Force             bool
	Detach            bool
	BranchName        string
}

type repo struct {
	ghrepo.Interface
	protocol string
}

func (r *repo) url() string {
	return ghrepo.FormatRemoteURL(r.Interface, r.protocol)
}

type cmds [][]string

func (cs *cmds) append(cmds cmds, err error) error {
	if err != nil {
		return err
	}
	*cs = append(*cs, cmds...)
	return nil
}

func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
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
	cmd.Flags().StringVarP(&opts.BranchName, "branch", "b", "", "Local branch name to use (default: the name of the head branch)")

	return cmd
}

func checkoutRun(opts *CheckoutOptions) error {
	var cmdQueue cmds

	pr, baseRepo, err := findPR(opts)
	if err != nil {
		return err
	}

	cmdQueue = append(cmdQueue, []string{"fetch", baseRepo.url(), fmt.Sprintf("refs/pull/%d/head", pr.Number)})

	err = cmdQueue.append(cmdsForCheckoutBranch(pr, baseRepo, opts))
	if err != nil {
		return err
	}

	if opts.RecurseSubmodules {
		cmdQueue = append(cmdQueue,
			[]string{"submodule", "sync", "--recursive"},
			[]string{"submodule", "update", "--init", "--recursive"},
		)
	}

	err = executeCmds(opts.GitClient, cmdQueue)

	return err
}

func findPR(opts *CheckoutOptions) (*api.PullRequest, *repo, error) {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number", "headRefName", "headRepository", "headRepositoryOwner", "isCrossRepository", "maintainerCanModify"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return nil, nil, err
	}

	if strings.HasPrefix(pr.HeadRefName, "-") {
		return nil, nil, fmt.Errorf("invalid branch name: %q", pr.HeadRefName)
	}

	cfg, err := opts.Config()
	if err != nil {
		return nil, nil, err
	}

	protocol, err := cfg.GetOrDefault(baseRepo.RepoHost(), "git_protocol")
	if err != nil {
		return nil, nil, err
	}

	return pr, &repo{Interface: baseRepo, protocol: protocol}, nil
}

func cmdsForCheckoutBranch(pr *api.PullRequest, baseRepo *repo, opts *CheckoutOptions) ([][]string, error) {
	var cmds cmds

	if opts.Detach {
		cmds = append(cmds, []string{"checkout", "--detach", "FETCH_HEAD"})
		return cmds, nil
	}

	localBranch, err := getLocalBranchName(pr, baseRepo, opts)
	if err != nil {
		return nil, err
	}

	if !localBranchExists(opts.GitClient, localBranch) {
		cmds = append(cmds, []string{"branch", localBranch, "FETCH_HEAD"})

		err = cmds.append(cmdsForConfigBranch(pr, baseRepo, opts, localBranch))
		if err != nil {
			return nil, err
		}
	}

	cmds = append(cmds, []string{"checkout", localBranch})

	if opts.Force {
		cmds = append(cmds, []string{"reset", "--hard", "FETCH_HEAD"})
	} else {
		// TODO: check if non-fast-forward and suggest to use `--force`
		cmds = append(cmds, []string{"merge", "--ff-only", "FETCH_HEAD"})
	}

	return cmds, nil
}

func getRepoURL(repo ghrepo.Interface, opts *CheckoutOptions) (string, error) {
	cfg, err := opts.Config()
	if err != nil {
		return "", err
	}

	protocol, err := cfg.GetOrDefault(repo.RepoHost(), "git_protocol")
	if err != nil {
		return "", err
	}

	return ghrepo.FormatRemoteURL(repo, protocol), nil
}

func getLocalBranchName(pr *api.PullRequest, baseRepo ghrepo.Interface, opts *CheckoutOptions) (string, error) {
	defaultBranch, err := getDefaultBranch(baseRepo, opts)
	if err != nil {
		return "", err
	}

	localBranch := pr.HeadRefName
	if opts.BranchName != "" {
		localBranch = opts.BranchName
	} else if pr.HeadRefName == defaultBranch {
		// avoid naming the new branch the same as the default branch
		localBranch = fmt.Sprintf("%s/%s", pr.HeadRepositoryOwner.Login, localBranch)
	}

	return localBranch, nil
}

func getDefaultBranch(repo ghrepo.Interface, opts *CheckoutOptions) (string, error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return "", err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	return api.RepoDefaultBranch(apiClient, repo)
}

func cmdsForUpdateLocalBranch(pr *api.PullRequest, baseRepoURL string, opts *CheckoutOptions, localBranch string) ([][]string, error) {
	var cmds [][]string

	if opts.Force {
		cmds = append(cmds, []string{"reset", "--hard", "FETCH_HEAD"})
	} else {
		// TODO: check if non-fast-forward and suggest to use `--force`
		cmds = append(cmds, []string{"merge", "--ff-only", "FETCH_HEAD"})
	}
	return cmds, nil
}

func cmdsForConfigBranch(pr *api.PullRequest, baseRepo *repo, opts *CheckoutOptions, localBranch string) ([][]string, error) {

	var cmds cmds

	remote := baseRepo.url()
	mergeRef := pr.HeadRefName

	if pr.MaintainerCanModify && pr.HeadRepository != nil {
		headRepo := ghrepo.NewWithHost(pr.HeadRepositoryOwner.Login, pr.HeadRepository.Name, baseRepo.RepoHost())
		var err error
		remote, err = getRepoURL(headRepo, opts)
		if err != nil {
			return nil, err
		}
		mergeRef = fmt.Sprintf("refs/heads/%s", pr.HeadRefName)
	}

	if missingMergeConfigForBranch(opts.GitClient, localBranch) {
		// .remote is needed for `git pull` to work
		// .pushRemote is needed for `git push` to work, if user has set `remote.pushDefault`.
		// see https://git-scm.com/docs/git-config#Documentation/git-config.txt-branchltnamegtremote
		cmds = append(cmds,
			[]string{"config", fmt.Sprintf("branch.%s.remote", localBranch), remote},
			[]string{"config", fmt.Sprintf("branch.%s.pushRemote", localBranch), remote},
			[]string{"config", fmt.Sprintf("branch.%s.merge", localBranch), mergeRef},
		)
	}

	return cmds, nil
}

func missingMergeConfigForBranch(client *git.Client, b string) bool {
	mc, err := client.Config(context.Background(), fmt.Sprintf("branch.%s.merge", b))
	return err != nil || mc == ""
}

func localBranchExists(client *git.Client, b string) bool {
	_, err := client.ShowRefs(context.Background(), []string{"refs/heads/" + b})
	return err == nil
}

func executeCmds(client *git.Client, cmdQueue [][]string) error {
	for _, args := range cmdQueue {
		var err error
		var cmd *git.Command
		if args[0] == "fetch" || args[0] == "submodule" {
			cmd, err = client.AuthenticatedCommand(context.Background(), args...)
		} else {
			cmd, err = client.Command(context.Background(), args...)
		}
		if err != nil {
			return err
		}
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
