package develop

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DevelopOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	IssueRepo     string
	IssueSelector string
	Name          string
	BaseBranch    string
	Checkout      bool
}

func NewCmdDevelop(f *cmdutil.Factory, runF func(*DevelopOptions) error) *cobra.Command {
	opts := &DevelopOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Browser:    f.Browser,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "develop",
		Short: "Manage branches for an issue",
		Example: heredoc.Doc(`
			$ gh issue develop --list 123 # list branches for issue 123
			$ gh issue develop --issue-repo "github/cli" 123 list branches for issue 123 in repo "github/cli"
			$ gh issue develop 123 --name "my-branch" --head main
			`),
		Args: cmdutil.ExactArgs(1, "issue number is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			opts.IssueSelector = args[0]
			return developRun(opts)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&opts.Name, "name", "n", "", "Name of the branch to create")
	fl.StringVarP(&opts.BaseBranch, "base-branch", "b", "", "Name of the base branch")
	fl.StringVarP(&opts.IssueRepo, "issue-repo", "i", "", "Name of the issue's repository")
	return cmd
}

func developRun(opts *DevelopOptions) (err error) {
	fmt.Printf("starting\n")
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "got the http client\n")
	apiClient := api.NewClientFromHTTP(httpClient)
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "got the baseRepo")
	opts.IO.StartProgressIndicator()
	fmt.Fprintf(opts.IO.ErrOut, "running")
	repo, err := api.GitHubRepo(apiClient, baseRepo)
	fmt.Fprintf(opts.IO.ErrOut, "found your repo %s\n", repo.Name)
	if err != nil {
		return err
	}

	oid, default_branch_oid, err := api.FindBaseOid(apiClient, repo, opts.BaseBranch)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "found %s for ref %s, and found default branch oid %s\n", oid, opts.BaseBranch, default_branch_oid)
	// get the id of the issue repo
	issue, _, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.IssueNumber, []string{"id", "number", "title", "state"})
	if err != nil {
		return err
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
		fmt.Fprintf(opts.IO.Out, "Created %s\n", ref.BranchName)
	}
	if err != nil {
		return err
	}
	return
}
