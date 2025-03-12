package checkout

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	cliContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CheckoutOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	IO         *iostreams.IOStreams
	Remotes    func() (cliContext.Remotes, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Config     func() (gh.Config, error)

	Force             bool
	RecurseSubmodules bool
	BranchName        string
	TagName           string
}

func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Remotes:    f.Remotes,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "checkout [<tag>]",
		Short: "Check out a release tag in git",
		Long: heredoc.Doc(`
			Check out a GitHub release tag in your local repository.

			Without a tag name, the latest release in the repository is checked out.
			Use the --repo flag to specify a repository other than the current one.
		`),
		Example: heredoc.Doc(`
			# Checkout the latest release
			$ gh release checkout

			# Checkout a specific release tag
			$ gh release checkout v2.67.0

			# Checkout a release from a non-local repo
			$ gh release checkout v2.67.0 --repo cli/cli

			# Checkout with a custom branch name
			$ gh release checkout v2.67.0 -b my-release-branch
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.TagName = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(opts)
		},
	}

	// Flags
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Force checkout, resetting any local branch to the release tag")
	cmd.Flags().BoolVarP(&opts.RecurseSubmodules, "recurse-submodules", "", false, "Update all submodules after checkout")
	cmd.Flags().StringVarP(&opts.BranchName, "branch", "b", "", "Local branch name to use (default: tag name)")

	return cmd
}

func checkoutRun(opts *CheckoutOptions) error {
	fmt.Println("fetching release...")
	return nil
}
