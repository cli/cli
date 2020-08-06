package close

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CloseOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string
}

func NewCmdClose(f *cmdutil.Factory, runF func(*CloseOptions) error) *cobra.Command {
	opts := &CloseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "close {<number> | <url> | <branch>}",
		Short: "Close a pull request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return closeRun(opts)
		},
	}

	return cmd
}

func closeRun(opts *CloseOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, baseRepo, err := shared.PRFromArgs(apiClient, opts.BaseRepo, nil, nil, opts.SelectorArg)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) can't be closed because it was already merged", utils.Red("!"), pr.Number, pr.Title)
		return cmdutil.SilentError
	} else if pr.Closed {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) is already closed\n", utils.Yellow("!"), pr.Number, pr.Title)
		return nil
	}

	err = api.PullRequestClose(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Closed pull request #%d (%s)\n", utils.Green("✔"), pr.Number, pr.Title)

	return nil
}
