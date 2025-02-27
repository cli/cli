package develop

import (
	ctx "context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DevelopOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)

	IssueSelector string
	Name          string
	BranchRepo    string
	BaseBranch    string
	Checkout      bool
	List          bool
}

func NewCmdDevelop(f *cmdutil.Factory, runF func(*DevelopOptions) error) *cobra.Command {
	opts := &DevelopOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "develop {<number> | <url>}",
		Short: "Manage linked branches for an issue",
		Long: heredoc.Docf(`
			Manage linked branches for an issue.

			When using the %[1]s--base%[1]s flag, the new development branch will be created from the specified
			remote branch. The new branch will be configured as the base branch for pull requests created using
			%[1]sgh pr create%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# List branches for issue 123
			$ gh issue develop --list 123

			# List branches for issue 123 in repo cli/cli
			$ gh issue develop --list --repo cli/cli 123

			# Create a branch for issue 123 based on the my-feature branch
			$ gh issue develop 123 --base my-feature

			# Create a branch for issue 123 and checkout it out
			$ gh issue develop 123 --checkout

			# Create a branch in repo monalisa/cli for issue 123 in repo cli/cli
			$ gh issue develop 123 --repo cli/cli --branch-repo monalisa/cli
		`),
		Args: cmdutil.ExactArgs(1, "issue number or url is required"),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// This is all a hack to not break the deprecated issue-repo flag.
			// It will be removed in the near future and this hack can be removed at the same time.
			flags := cmd.Flags()
			if flags.Changed("issue-repo") {
				if flags.Changed("repo") {
					if flags.Changed("branch-repo") {
						return cmdutil.FlagErrorf("specify only `--repo` and `--branch-repo`")
					}
					branchRepo, _ := flags.GetString("repo")
					_ = flags.Set("branch-repo", branchRepo)
				}
				repo, _ := flags.GetString("issue-repo")
				_ = flags.Set("repo", repo)
			}
			if cmd.Parent() != nil {
				return cmd.Parent().PersistentPreRunE(cmd, args)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.IssueSelector = args[0]
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--branch-repo`", opts.List, opts.BranchRepo != ""); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--base`", opts.List, opts.BaseBranch != ""); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--checkout`", opts.List, opts.Checkout); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of `--list` or `--name`", opts.List, opts.Name != ""); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return developRun(opts)
		},
	}

	fl := cmd.Flags()
	fl.StringVar(&opts.BranchRepo, "branch-repo", "", "Name or URL of the repository where you want to create your new branch")
	fl.StringVarP(&opts.BaseBranch, "base", "b", "", "Name of the remote branch you want to make your new branch from")
	fl.BoolVarP(&opts.Checkout, "checkout", "c", false, "Checkout the branch after creating it")
	fl.BoolVarP(&opts.List, "list", "l", false, "List linked branches for the issue")
	fl.StringVarP(&opts.Name, "name", "n", "", "Name of the branch to create")

	var issueRepoSelector string
	fl.StringVarP(&issueRepoSelector, "issue-repo", "i", "", "Name or URL of the issue's repository")
	_ = cmd.Flags().MarkDeprecated("issue-repo", "use `--repo` instead")

	return cmd
}

func developRun(opts *DevelopOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	issue, issueRepo, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.IssueSelector, []string{"id", "number"})
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	opts.IO.StartProgressIndicator()
	err = api.CheckLinkedBranchFeature(apiClient, issueRepo.RepoHost())
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.List {
		return developRunList(opts, apiClient, issueRepo, issue)
	}
	return developRunCreate(opts, apiClient, issueRepo, issue)
}

func developRunCreate(opts *DevelopOptions, apiClient *api.Client, issueRepo ghrepo.Interface, issue *api.Issue) error {
	branchRepo := issueRepo
	var repoID string
	if opts.BranchRepo != "" {
		var err error
		branchRepo, err = ghrepo.FromFullName(opts.BranchRepo)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	repoID, branchID, err := api.FindRepoBranchID(apiClient, branchRepo, opts.BaseBranch)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	branchName, err := api.CreateLinkedBranch(apiClient, branchRepo.RepoHost(), repoID, issue.ID, branchID, opts.Name)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	// Remember which branch to target when creating a PR.
	if opts.BaseBranch != "" {
		err = opts.GitClient.SetBranchConfig(ctx.Background(), branchName, git.MergeBaseConfig, opts.BaseBranch)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(opts.IO.Out, "%s/%s/tree/%s\n", branchRepo.RepoHost(), ghrepo.FullName(branchRepo), branchName)

	return checkoutBranch(opts, branchRepo, branchName)
}

func developRunList(opts *DevelopOptions, apiClient *api.Client, issueRepo ghrepo.Interface, issue *api.Issue) error {
	opts.IO.StartProgressIndicator()
	branches, err := api.ListLinkedBranches(apiClient, issueRepo, issue.Number)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if len(branches) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("no linked branches found for %s#%d", ghrepo.FullName(issueRepo), issue.Number))
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "\nShowing linked branches for %s#%d\n\n", ghrepo.FullName(issueRepo), issue.Number)
	}

	printLinkedBranches(opts.IO, branches)

	return nil
}

func printLinkedBranches(io *iostreams.IOStreams, branches []api.LinkedBranch) {
	cs := io.ColorScheme()
	table := tableprinter.New(io, tableprinter.WithHeader("BRANCH", "URL"))
	for _, branch := range branches {
		table.AddField(branch.BranchName, tableprinter.WithColor(cs.ColorFromString("cyan")))
		table.AddField(branch.URL)
		table.EndRow()
	}
	_ = table.Render()
}

func checkoutBranch(opts *DevelopOptions, branchRepo ghrepo.Interface, checkoutBranch string) (err error) {
	remotes, err := opts.Remotes()
	if err != nil {
		// If the user specified the branch to be checked out and no remotes are found
		// display an error. Otherwise bail out silently, likely the command was not
		// run from inside a git directory.
		if opts.Checkout {
			return err
		} else {
			return nil
		}
	}

	baseRemote, err := remotes.FindByRepo(branchRepo.RepoOwner(), branchRepo.RepoName())
	if err != nil {
		// If the user specified the branch to be checked out and no remote matches the
		// base repo, then display an error. Otherwise bail out silently.
		if opts.Checkout {
			return err
		} else {
			return nil
		}
	}

	gc := opts.GitClient

	if err := gc.Fetch(ctx.Background(), baseRemote.Name, fmt.Sprintf("+refs/heads/%[1]s:refs/remotes/%[2]s/%[1]s", checkoutBranch, baseRemote.Name)); err != nil {
		return err
	}

	if !opts.Checkout {
		return nil
	}

	if gc.HasLocalBranch(ctx.Background(), checkoutBranch) {
		if err := gc.CheckoutBranch(ctx.Background(), checkoutBranch); err != nil {
			return err
		}

		if err := gc.Pull(ctx.Background(), baseRemote.Name, checkoutBranch); err != nil {
			_, _ = fmt.Fprintf(opts.IO.ErrOut, "%s warning: not possible to fast-forward to: %q\n", opts.IO.ColorScheme().WarningIcon(), checkoutBranch)
		}
	} else {
		if err := gc.CheckoutNewBranch(ctx.Background(), baseRemote.Name, checkoutBranch); err != nil {
			return err
		}
	}

	return nil
}
