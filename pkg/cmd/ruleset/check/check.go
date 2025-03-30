package check

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CheckOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Git        *git.Client
	Browser    browser.Browser

	Branch  string
	Default bool
	WebMode bool
}

func NewCmdCheck(f *cmdutil.Factory, runF func(*CheckOptions) error) *cobra.Command {
	opts := &CheckOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Git:        f.GitClient,
	}
	cmd := &cobra.Command{
		Use:   "check [<branch>]",
		Short: "View rules that would apply to a given branch",
		Long: heredoc.Docf(`
			View information about GitHub rules that apply to a given branch.

			The provided branch name does not need to exist; rules will be displayed that would apply
			to a branch with that name. All rules are returned regardless of where they are configured.

			If no branch name is provided, then the current branch will be used.

			The %[1]s--default%[1]s flag can be used to view rules that apply to the default branch of the
			repository.
		`, "`"),
		Example: heredoc.Doc(`
			# View all rules that apply to the current branch
			$ gh ruleset check

			# View all rules that apply to a branch named "my-branch" in a different repository
			$ gh ruleset check my-branch --repo owner/repo

			# View all rules that apply to the default branch in a different repository
			$ gh ruleset check --default --repo owner/repo

			# View a ruleset configured in a different repository or any of its parents
			$ gh ruleset view 23 --repo owner/repo

			# View an organization-level ruleset
			$ gh ruleset view 23 --org my-org
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.Branch = args[0]
			}

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--default` or a branch name",
				opts.Branch != "",
				opts.Default,
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return checkRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Default, "default", false, "Check rules on default branch")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the branch rules page in a web browser")

	return cmd
}

func checkRun(opts *CheckOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	repoI, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine repo to use: %w", err)
	}

	git := opts.Git

	if opts.Default {
		repo, err := api.GitHubRepo(client, repoI)
		if err != nil {
			return fmt.Errorf("could not get repository information: %w", err)
		}
		opts.Branch = repo.DefaultBranchRef.Name
	}

	if opts.Branch == "" {
		opts.Branch, err = git.CurrentBranch(context.Background())
		if err != nil {
			return fmt.Errorf("could not determine current branch: %w", err)
		}
	}

	if opts.WebMode {
		// the query string parameter may have % signs in it, so it must be carefully used with Printf functions
		queryString := fmt.Sprintf("?ref=%s", url.QueryEscape("refs/heads/"+opts.Branch))
		rawUrl := ghrepo.GenerateRepoURL(repoI, "rules")

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(rawUrl))
		}

		return opts.Browser.Browse(rawUrl + queryString)
	}

	var rules []shared.RulesetRule

	endpoint := fmt.Sprintf("repos/%s/%s/rules/branches/%s", repoI.RepoOwner(), repoI.RepoName(), url.PathEscape(opts.Branch))

	if err = client.REST(repoI.RepoHost(), "GET", endpoint, nil, &rules); err != nil {
		return fmt.Errorf("GET %s failed: %w", endpoint, err)
	}

	w := opts.IO.Out

	fmt.Fprintf(w, "%d rules apply to branch %s in repo %s/%s\n", len(rules), opts.Branch, repoI.RepoOwner(), repoI.RepoName())

	if len(rules) > 0 {
		fmt.Fprint(w, "\n")
		fmt.Fprint(w, shared.ParseRulesForDisplay(rules))
	}

	return nil
}
