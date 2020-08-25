package run

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type RunOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
	Remotes    func() (context.Remotes, error)

	SelectorArg string
	Failed      bool
	All         bool
	Check       string
	Suite       string
}

func NewCmdRun(f *cmdutil.Factory, runF func(*RunOptions) error) *cobra.Command {
	opts := &RunOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Branch:     f.Branch,
		Remotes:    f.Remotes,
		BaseRepo:   f.BaseRepo,
	}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run checks for a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return runRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Failed, "failed", "f", false, "Rerun all failed checks")
	cmd.Flags().BoolVarP(&opts.All, "all", "a", false, "Rerun all checks")
	cmd.Flags().StringVarP(&opts.Check, "check", "c", "", "Rerun a check by name")
	cmd.Flags().StringVarP(&opts.Suite, "suite", "s", "", "Rerun all checks in a suite")

	return cmd
}

func runRun(opts *RunOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, _, err := shared.PRFromArgs(apiClient, opts.BaseRepo, opts.Branch, opts.Remotes, opts.SelectorArg)
	if err != nil {
		return err
	}

	fmt.Printf("DEBUG %#v\n", pr)
	fmt.Printf("DEBUG %#v\n", repo)

	// gh pr run
	// => interactive multi select of checks to re-run
	// gh pr run --failed
	// gh pr run --all
	// gh pr run --suite="foo"
	// gh pr run --check="foo"

	return nil
}
