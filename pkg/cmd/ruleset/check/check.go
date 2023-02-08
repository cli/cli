package check

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CheckOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Git        *git.Client

	Branch  string
	Default bool
}

func NewCmdCheck(f *cmdutil.Factory, runF func(*CheckOptions) error) *cobra.Command {
	opts := &CheckOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Git:        f.GitClient,
	}
	cmd := &cobra.Command{
		Use:   "check [<branch>]",
		Short: "Print rules that would apply to a given branch",
		Long: heredoc.Doc(`
			TODO
		`),
		Example: "TODO",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			// TODO flag to do a push

			if len(args) > 0 {
				opts.Branch = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			if opts.Branch != "" && opts.Default {
				return cmdutil.FlagErrorf(
					"branch argument '%s' and --default mutually exclusive", opts.Branch)
			}

			return checkRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Default, "default", false, "Check rules on default branch")

	return cmd
}

func checkRun(opts *CheckOptions) error {
	// TODO ask about pushing (if interactive)
	// TODO error if not interactive and --push not specified
	// TODO parsing for errors on push

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	repoI, err := opts.BaseRepo()
	if err != nil {
		return err
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

	var lol interface{}

	endpoint := fmt.Sprintf("repos/%s/%s/rules/branches/%s", repoI.RepoOwner(), repoI.RepoName(), url.PathEscape(opts.Branch))

	if err = client.REST(repoI.RepoHost(), "GET", endpoint, nil, &lol); err != nil {
		return fmt.Errorf("GET %s failed: %w", endpoint, err)
	}

	// TODO handle 404s gracefully
	// TODO actually parse JSON

	fmt.Printf("DBG %#v\n", lol)

	return nil
}
