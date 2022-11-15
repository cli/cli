package develop

import (
	ctx "context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
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
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)

	IssueRepoSelector string
	IssueSelector     string
	Name              string
	BaseBranch        string
	Checkout          bool
	List              bool
}

func NewCmdDevelop(f *cmdutil.Factory, runF func(*DevelopOptions) error) *cobra.Command {
	opts := &DevelopOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
		Remotes:    f.Remotes,
	}

	cmd := &cobra.Command{
		Use:   "develop [flags] {<number> | <url>}",
		Short: "Manage linked branches for an issue",
		Example: heredoc.Doc(`
			$ gh issue develop --list 123 # list branches for issue 123
			$ gh issue develop --list --issue-repo "github/cli" 123 # list branches for issue 123 in repo "github/cli"
			$ gh issue develop --list https://github.com/github/cli/issues/123 # list branches for issue 123 in repo "github/cli"
			$ gh issue develop 123 --name "my-branch" --base my-feature # create a branch for issue 123 based on the my-feature branch
			$ gh issue develop 123 --checkout # fetch and checkout the branch for issue 123 after creating it
			`),
		Args: cmdutil.ExactArgs(1, "issue number or url is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			opts.IssueSelector = args[0]
			if opts.List {
				return developRunList(opts)
			}
			return developRunCreate(opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.BaseBranch, "base", "b", "", "Name of the base branch you want to make your new branch from")
	fl.BoolVarP(&opts.Checkout, "checkout", "c", false, "Checkout the branch after creating it")
	fl.StringVarP(&opts.IssueRepoSelector, "issue-repo", "i", "", "Name or URL of the issue's repository")
	fl.BoolVarP(&opts.List, "list", "l", false, "List linked branches for the issue")
	fl.StringVarP(&opts.Name, "name", "n", "", "Name of the branch to create")
	return cmd
}

func developRunCreate(opts *DevelopOptions) (err error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	err = api.CheckLinkedBranchFeature(apiClient, baseRepo.RepoHost())
	if err != nil {
		return err
	}

	repo, err := api.GitHubRepo(apiClient, baseRepo)
	if err != nil {
		return err
	}

	issueNumber, issueRepo, err := issueMetadata(opts.IssueSelector, opts.IssueRepoSelector, baseRepo)
	if err != nil {
		return err
	}

	// The mutation requires the issue id, not just its number
	issue, _, err := shared.IssueFromArgWithFields(httpClient, func() (ghrepo.Interface, error) { return issueRepo, nil }, fmt.Sprint(issueNumber), []string{"id"})
	if err != nil {
		return err
	}

	// The mutation takes an oid instead of a branch name as it's a more stable reference
	oid, default_branch_oid, err := api.FindBaseOid(apiClient, repo, opts.BaseBranch)
	if err != nil {
		return err
	}

	if oid == "" {
		oid = default_branch_oid
	}

	// get the oid of the branch from the base repo
	params := map[string]interface{}{
		"issueId":      issue.ID,
		"name":         opts.Name,
		"oid":          oid,
		"repositoryId": repo.ID,
	}

	ref, err := api.CreateBranchIssueReference(apiClient, repo, params)
	opts.IO.StopProgressIndicator()
	if ref != nil {
		baseRepo.RepoHost()
		fmt.Fprintf(opts.IO.Out, "%s/%s/%s/tree/%s\n", baseRepo.RepoHost(), baseRepo.RepoOwner(), baseRepo.RepoName(), ref.BranchName)

		if opts.Checkout {
			return checkoutBranch(opts, baseRepo, ref.BranchName)
		}
	}
	if err != nil {
		return err
	}
	return
}

// If the issue is in the base repo, we can use the issue number directly. Otherwise, we need to use the issue's url or the IssueRepoSelector argument.
// If the repo from the URL doesn't match the IssueRepoSelector argument, we error.
func issueMetadata(issueSelector string, issueRepoSelector string, baseRepo ghrepo.Interface) (issueNumber int, issueFlagRepo ghrepo.Interface, err error) {
	var targetRepo ghrepo.Interface
	if issueRepoSelector != "" {
		issueFlagRepo, err = ghrepo.FromFullNameWithHost(issueRepoSelector, baseRepo.RepoHost())
		if err != nil {
			return 0, nil, err
		}
	}

	if issueFlagRepo != nil {
		targetRepo = issueFlagRepo
	}

	issueNumber, issueArgRepo, err := shared.IssueNumberAndRepoFromArg(issueSelector)
	if err != nil {
		return 0, nil, err
	}

	if issueArgRepo != nil {
		targetRepo = issueArgRepo

		if issueFlagRepo != nil {
			differentOwner := (issueFlagRepo.RepoOwner() != issueArgRepo.RepoOwner())
			differentName := (issueFlagRepo.RepoName() != issueArgRepo.RepoName())
			if differentOwner || differentName {
				return 0, nil, fmt.Errorf("issue repo in url %s/%s does not match the repo from --issue-repo %s/%s", issueArgRepo.RepoOwner(), issueArgRepo.RepoName(), issueFlagRepo.RepoOwner(), issueFlagRepo.RepoName())
			}
		}
	}

	if issueFlagRepo == nil && issueArgRepo == nil {
		targetRepo = baseRepo
	}

	if targetRepo == nil {
		return 0, nil, fmt.Errorf("could not determine issue repo")
	}

	return issueNumber, targetRepo, nil
}

func printLinkedBranches(io *iostreams.IOStreams, branches []api.LinkedBranch) {

	cs := io.ColorScheme()
	table := tableprinter.New(io)

	for _, branch := range branches {
		table.AddField(branch.BranchName, tableprinter.WithColor(cs.ColorFromString("cyan")))
		if io.CanPrompt() {
			table.AddField(branch.Url())
		}
		table.EndRow()
	}

	_ = table.Render()
}

func developRunList(opts *DevelopOptions) (err error) {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()

	err = api.CheckLinkedBranchFeature(apiClient, baseRepo.RepoHost())
	if err != nil {
		return err
	}
	issueNumber, issueRepo, err := issueMetadata(opts.IssueSelector, opts.IssueRepoSelector, baseRepo)
	if err != nil {
		return err
	}

	branches, err := api.ListLinkedBranches(apiClient, issueRepo, issueNumber)
	if err != nil {
		return err
	}

	opts.IO.StopProgressIndicator()
	if len(branches) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("no linked branches found for %s/%s#%d", issueRepo.RepoOwner(), issueRepo.RepoName(), issueNumber))
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "\nShowing linked branches for %s/%s#%d\n\n", issueRepo.RepoOwner(), issueRepo.RepoName(), issueNumber)
	}

	printLinkedBranches(opts.IO, branches)

	return nil

}

func checkoutBranch(opts *DevelopOptions, baseRepo ghrepo.Interface, checkoutBranch string) (err error) {
	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}

	baseRemote, err := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())
	if err != nil {
		return err
	}

	if opts.GitClient.HasLocalBranch(ctx.Background(), checkoutBranch) {
		if err := opts.GitClient.CheckoutBranch(ctx.Background(), checkoutBranch); err != nil {
			return err
		}
	} else {
		err := opts.GitClient.Fetch(ctx.Background(), "origin", fmt.Sprintf("+refs/heads/%[1]s:refs/remotes/origin/%[1]s", checkoutBranch))
		if err != nil {
			return err
		}

		if err := opts.GitClient.CheckoutNewBranch(ctx.Background(), baseRemote.Name, checkoutBranch); err != nil {
			return err
		}
	}

	if err := opts.GitClient.Pull(ctx.Background(), baseRemote.Name, checkoutBranch); err != nil {
		_, _ = fmt.Fprintf(opts.IO.ErrOut, "%s warning: not possible to fast-forward to: %q\n", opts.IO.ColorScheme().WarningIcon(), checkoutBranch)
	}

	return nil
}
