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

	return nil
}
