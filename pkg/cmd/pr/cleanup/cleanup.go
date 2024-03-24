package cleanup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CleanupOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	BaseRepo   func() (ghrepo.Interface, error)
	IO         *iostreams.IOStreams
	Prompter   prompter.Prompter

	Finder shared.PRFinder

	SelectorArg  string
	All          bool
	Strict       bool
	MergedOnly   bool
	UpToDateOnly bool
	Yes          bool
}

func NewCmdCleanup(f *cmdutil.Factory, runF func(*CleanupOptions) error) *cobra.Command {
	opts := &CleanupOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		BaseRepo:   f.BaseRepo,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "cleanup {<number> | <url> | <branch> | --all}",
		Short: "Clean up local branches of merged or closed pull requests",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return cleanupRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.All, "all", "", false, "Clean up all merged pull requests")
	cmd.Flags().BoolVarP(&opts.Strict, "strict", "", false, "Both --exclude-closed and --exclude-behind")
	cmd.Flags().BoolVarP(&opts.MergedOnly, "exclude-closed", "", false, "Exclude branches of closed pull requests")
	cmd.Flags().BoolVarP(&opts.UpToDateOnly, "exclude-behind", "", false, "Exclude branches that are behind their remote")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "", false, "Skip deletion confirmation")

	return cmd
}

func cleanupRun(opts *CleanupOptions) error {
	// Validate input arguments: --all and PR selector are mutually exclusive, but
	// at least one must be set.
	if opts.All && opts.SelectorArg != "" {
		return errors.New("Invalid arguments: cannot set both PR and --all")
	} else if opts.SelectorArg == "" && !opts.All {
		return errors.New("Invalid arguments: must set either PR or --all")
	}

	// Set flags.
	if opts.Strict {
		opts.MergedOnly = true
		opts.UpToDateOnly = true
	}

	if opts.All {
		// Get all local branches and their upstreams.
		ctx := context.Background()
		localBranches := opts.GitClient.LocalBranches(ctx)
		var branchesWithUpstream []git.Branch
		for _, localBranch := range localBranches {
			if localBranch.Upstream.RemoteName == "" {
				continue
			}
			branchesWithUpstream = append(branchesWithUpstream, localBranch)
		}

		// Get PRs associated with upstream branches.
		opts.IO.StartProgressIndicatorWithLabel(
			fmt.Sprintf(
				"Loading PRs for %d local branches with upstreams. This can take a minute if you have many branches...\n",
				len(branchesWithUpstream),
			),
		)

		var prs []*api.PullRequest
		prStates := []string{"MERGED", "CLOSED"}
		if opts.MergedOnly {
			prStates = []string{"MERGED"}
		}
		for _, branch := range branchesWithUpstream {
			// TODO: Can these be loaded in parallel?
			// TODO: This causes the progress indicator to "reset" very frequently.
			// Should the Finder itself have a progress indicator? Perhaps we should
			// invert that so consumers have control of the indicator instead.
			pr, _, err := opts.Finder.Find(shared.FindOptions{
				Selector: branch.Upstream.BranchName,
				Fields:   []string{"headRefOid", "headRefName", "title", "state"},
				States:   prStates,
			})
			if _, ok := err.(*shared.NotFoundError); ok {
				continue
			}
			if err != nil {
				return err
			} else {
				prs = append(prs, pr)
			}
		}
		opts.IO.StopProgressIndicator()

		// Reorganize branches for fast lookup.
		branchesByRemoteBranchName := make(map[string]git.Branch)
		for _, branch := range branchesWithUpstream {
			branchesByRemoteBranchName[branch.Upstream.BranchName] = branch
		}

		// Get the list of candidate branch deletions.
		//
		// Any local branch whose HEAD is a commit of a merged or closed PR is a
		// candidate for deletion, because the local branch's history is a prefix of
		// the remote branch's history (i.e. there are no local commits that the
		// upstream does not have).
		//
		// This behavior is altered by:
		// * --exclude-behind: the local branch's head ref must be the PR's head ref.
		// * --exclude-closed: closed PRs are not considered.
		deletionCandidates := make(map[git.Branch]*api.PullRequest)
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		repo, err := opts.BaseRepo()
		if err != nil {
			return err
		}
		for _, pr := range prs {
			branch := branchesByRemoteBranchName[pr.HeadRefName]
			isCandidate, err := branchIsDeletionCandidate(opts, httpClient, repo, branch, pr)
			if err != nil {
				return err
			}
			if isCandidate {
				deletionCandidates[branch] = pr
			}
		}

		return promptAndDeleteBranches(opts, deletionCandidates)
	} else {
		// Find the selected PR.
		prStates := []string{"MERGED", "CLOSED"}
		if opts.MergedOnly {
			prStates = []string{"MERGED"}
		}
		pr, _, err := opts.Finder.Find(shared.FindOptions{
			Selector: opts.SelectorArg,
			Fields:   []string{"headRefOid", "headRefName", "title", "state"},
			States:   prStates,
		})
		if err != nil {
			return err
		}

		// Find the relevant local branch.
		ctx := context.Background()
		var localBranch *git.Branch
		localBranches := opts.GitClient.LocalBranches(ctx)
		for _, b := range localBranches {
			if b.Upstream.BranchName == pr.HeadRefName {
				localBranch = &b
			}
		}
		if localBranch == nil {
			return errors.New("Could not find local branch for specified pull request.")
		}

		// Determine local branch candidacy.
		deletionCandidates := make(map[git.Branch]*api.PullRequest)
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}
		repo, err := opts.BaseRepo()
		if err != nil {
			return err
		}
		isCandidate, err := branchIsDeletionCandidate(opts, httpClient, repo, *localBranch, pr)
		if err != nil {
			return err
		}
		if isCandidate {
			deletionCandidates[*localBranch] = pr
		}

		return promptAndDeleteBranches(opts, deletionCandidates)
	}
}

var PRNotFound = errors.New("no merged pull requests found for commit")

func mergedPRAssociatedWithCommit(httpClient *http.Client, repo ghrepo.Interface, commit string) (*api.PullRequest, error) {
	type response struct {
		Repository struct {
			Object struct {
				AssociatedPullRequests struct {
					Nodes []api.PullRequest
				}
			}
		}
	}

	query := `
	query PullRequestForCommit($owner: String!, $repo: String!, $commitOid: GitObjectID!) {
		repository(owner: $owner, name: $repo) {
			object(oid: $commitOid) {
				... on Commit {
					associatedPullRequests(first: 30, orderBy: { field: CREATED_AT, direction: DESC }) {
						nodes {
							number
							state
							title
							headRefOid
						}
					}
				}
			}
		}
	}`

	variables := map[string]interface{}{
		"owner":     repo.RepoOwner(),
		"repo":      repo.RepoName(),
		"commitOid": commit,
	}

	var resp response
	client := api.NewClientFromHTTP(httpClient)
	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return nil, err
	}

	prs := resp.Repository.Object.AssociatedPullRequests.Nodes
	sort.SliceStable(prs, func(a, b int) bool {
		return prs[a].State == "MERGED" && prs[b].State != "MERGED"
	})

	for _, pr := range prs {
		if pr.State == "MERGED" {
			return &pr, nil
		}
	}

	return nil, PRNotFound
}

func branchIsDeletionCandidate(opts *CleanupOptions, httpClient *http.Client, repo ghrepo.Interface, branch git.Branch, pr *api.PullRequest) (bool, error) {
	if opts.MergedOnly && pr.State == "CLOSED" {
		return false, nil
	}

	// First, check to see if the branch of the PR is at the PR's head.
	if branch.Local.Hash == pr.HeadRefOid {
		return true, nil
	}

	// Otherwise, check if the branch of the PR is behind a PR's head. To do
	// this, we try to find a merged PR whose history contains the branch's
	// commit.
	if !opts.UpToDateOnly {
		pr, err := mergedPRAssociatedWithCommit(httpClient, repo, branch.Local.Hash)
		if err == PRNotFound {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return pr.State == "MERGED", nil
	}

	return false, nil
}

func promptAndDeleteBranches(opts *CleanupOptions, deletionCandidates map[git.Branch]*api.PullRequest) error {
	// Interactively confirm branch deletion.
	cs := opts.IO.ColorScheme()
	if len(deletionCandidates) == 0 {
		fmt.Fprintf(opts.IO.Out, "%s No branches to be cleaned up!\n", cs.SuccessIcon())
		return nil
	}

	var branchesInAlphaOrder []git.Branch
	for branch := range deletionCandidates {
		branchesInAlphaOrder = append(branchesInAlphaOrder, branch)
	}
	sort.Slice(branchesInAlphaOrder, func(i, j int) bool {
		return branchesInAlphaOrder[i].Local.Name < branchesInAlphaOrder[j].Local.Name
	})

	fmt.Fprintf(opts.IO.Out, "\nThe following branches can be cleaned up:\n\n")
	table := tableprinter.New(opts.IO)
	table.HeaderRow("Branch", "Status", "Pull Request")
	for _, branch := range branchesInAlphaOrder {
		pr := deletionCandidates[branch]

		table.AddField(branch.Local.Name)

		state := pr.State
		if branch.Local.Hash != pr.HeadRefOid {
			state = cs.WarningIcon() + " " + cs.Yellow(state)
		}
		if state == "MERGED" {
			state = cs.SuccessIcon() + " " + cs.Green(state)
		} else if state == "CLOSED" {
			state = cs.SuccessIcon() + " " + cs.Red(state)
		}
		table.AddField(state)

		table.AddField(
			fmt.Sprintf(
				"%s %s",
				cs.Grayf("#%d", pr.Number),
				pr.Title,
			),
		)

		table.EndRow()
	}
	err := table.Render()
	if err != nil {
		return err
	}

	if !opts.UpToDateOnly {
		fmt.Fprintf(opts.IO.Out, "\n%s indicates that a local branch is behind its remote.\n", cs.WarningIcon())
	}
	fmt.Fprintf(opts.IO.Out, "\n")

	confirmed := false
	if opts.Yes {
		confirmed = true
	} else if opts.IO.CanPrompt() {
		branchTypeStr := "merged or closed"
		if opts.MergedOnly {
			branchTypeStr = "merged"
		}
		confirmed, err = opts.Prompter.Confirm(
			fmt.Sprintf("Delete all %d %s branches?", len(deletionCandidates), branchTypeStr),
			false,
		)
		if err != nil {
			return err
		}
	}
	// Delete branches.
	if confirmed {
		ctx := context.Background()
		for branch := range deletionCandidates {
			err := opts.GitClient.DeleteLocalBranch(ctx, branch.Local.Name)
			if err != nil {
				return err
			}
		}
		fmt.Fprintf(opts.IO.Out, "Deleted %d branches.\n", len(deletionCandidates))
	} else {
		fmt.Fprintf(opts.IO.Out, "Not deleting any branches.\n")
	}

	return nil
}
