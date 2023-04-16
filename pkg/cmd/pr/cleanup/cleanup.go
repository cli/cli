package cleanup

import (
	"net/http"

	cliContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CleanupOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	Remotes    func() (cliContext.Remotes, error)
	Branch     func() (string, error)

	Finder shared.PRFinder

	SelectorArg string
	All         bool
}

func NewCmdCleanup(f *cmdutil.Factory, runF func(*CleanupOptions) error) *cobra.Command {
	opts := &CleanupOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "cleanup {<number> | <url> | <branch> | --all}",
		Short: "Clean up local branches of merged pull requests",
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

	return cmd
}

func cleanupRun(opts *CleanupOptions) error {
	return nil
}
