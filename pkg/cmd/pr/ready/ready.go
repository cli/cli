package ready

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ReadyOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Branch     func() (string, error)

	SelectorArg string
}

func NewCmdReady(f *cmdutil.Factory, runF func(*ReadyOptions) error) *cobra.Command {
	opts := &ReadyOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Remotes:    f.Remotes,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "ready [<number> | <url> | <branch>]",
		Short: "Mark a pull request as ready for review",
		Long: heredoc.Doc(`
			Mark a pull request as ready for review

			Without an argument, the pull request that belongs to the current branch
			is displayed.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return &cmdutil.FlagError{Err: errors.New("argument required when using the --repo flag")}
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return readyRun(opts)
		},
	}

	return cmd
}

func readyRun(opts *ReadyOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, baseRepo, err := shared.PRFromArgs(apiClient, opts.BaseRepo, opts.Branch, opts.Remotes, opts.SelectorArg)
	if err != nil {
		return err
	}

	if pr.Closed {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is closed. Only draft pull requests can be marked as \"ready for review\"", cs.Red("!"), pr.Number)
		return cmdutil.SilentError
	} else if !pr.IsDraft {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is already \"ready for review\"\n", cs.Yellow("!"), pr.Number)
		return nil
	}

	err = api.PullRequestReady(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is marked as \"ready for review\"\n", cs.SuccessIconWithColor(cs.Green), pr.Number)

	return nil
}
