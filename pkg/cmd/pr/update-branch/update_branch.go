package update_branch

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	shared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type UpdateBranchOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	GitClient  *git.Client

	Finder shared.PRFinder

	SelectorArg string
	Rebase      bool
}

func NewCmdUpdateBranch(f *cmdutil.Factory, runF func(*UpdateBranchOptions) error) *cobra.Command {
	opts := &UpdateBranchOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
	}

	cmd := &cobra.Command{
		Use:   "update-branch [<number> | <url> | <branch>]",
		Short: "Update a pull request branch",
		Long: heredoc.Docf(`
			Update a pull request branch with latest changes of the base branch.

			Without an argument, the pull request that belongs to the current branch is selected.

			The default behavior is to update with a merge commit (i.e., merging the base branch
			into the the PR's branch). To reconcile the changes with rebasing on top of the base
			branch, the %[1]s--rebase%[1]s option should be provided.
		`, "`"),
		Example: heredoc.Doc(`
			$ gh pr update-branch 23
			$ gh pr update-branch 23 --rebase
			$ gh pr update-branch 23 --repo owner/repo
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return updateBranchRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Rebase, "rebase", false, "Update PR branch by rebasing on top of latest base branch")

	return cmd
}

func updateBranchRun(opts *UpdateBranchOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "number", "headRefName", "headRefOid", "headRepositoryOwner", "mergeable"},
	}

	pr, repo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	if pr.Mergeable == api.PullRequestMergeableConflicting {
		fmt.Fprintf(opts.IO.ErrOut, "%s Cannot update PR branch due to conflicts\n", cs.FailureIcon())
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	headRef := pr.HeadRefName
	if pr.HeadRepositoryOwner.Login != repo.RepoOwner() {
		// Only prepend the `owner:` to comparison head ref if the PR head
		// branch is on another repo (e.g., a fork).
		headRef = pr.HeadRepositoryOwner.Login + ":" + headRef
	}

	opts.IO.StartProgressIndicator()
	comparison, err := api.ComparePullRequestBaseBranchWith(apiClient, repo, pr.Number, headRef)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if comparison.BehindBy == 0 {
		// The PR branch is not behind the base branch, so it'a already up-to-date.
		fmt.Fprintf(opts.IO.Out, "%s PR branch already up-to-date\n", cs.SuccessIcon())
		return nil
	}

	opts.IO.StartProgressIndicator()
	err = updatePullRequestBranch(apiClient, repo, pr.ID, pr.HeadRefOid, opts.Rebase)
	opts.IO.StopProgressIndicator()
	if err != nil {
		// TODO: this is a best effort approach and not a resilient way of handling API errors.
		if strings.Contains(err.Error(), "GraphQL: merge conflict between base and head (updatePullRequestBranch)") {
			fmt.Fprintf(opts.IO.ErrOut, "%s Cannot update PR branch due to conflicts\n", cs.FailureIcon())
			return cmdutil.SilentError
		}
		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s PR branch updated\n", cs.SuccessIcon())
	return nil
}

// updatePullRequestBranch calls the GraphQL API endpoint to update the given PR
// branch with latest changes of its base.
func updatePullRequestBranch(apiClient *api.Client, repo ghrepo.Interface, pullRequestID string, expectedHeadOid string, rebase bool) error {
	updateMethod := githubv4.PullRequestBranchUpdateMethodMerge
	if rebase {
		updateMethod = githubv4.PullRequestBranchUpdateMethodRebase
	}

	params := githubv4.UpdatePullRequestBranchInput{
		PullRequestID:   pullRequestID,
		ExpectedHeadOid: (*githubv4.GitObjectID)(&expectedHeadOid),
		UpdateMethod:    &updateMethod,
	}

	return api.UpdatePullRequestBranch(apiClient, repo, params)
}
