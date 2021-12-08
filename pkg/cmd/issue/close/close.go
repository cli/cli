package close

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/frecency"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CloseOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string

	Frecency    *frecency.Manager
	UseFrecency bool
	Now         func() time.Time
}

func NewCmdClose(f *cmdutil.Factory, runF func(*CloseOptions) error) *cobra.Command {
	opts := &CloseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Frecency:   f.FrecencyManager,
	}

	cmd := &cobra.Command{
		Use:   "close {<number> | <url>}",
		Short: "Close issue",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			} else if opts.IO.CanPrompt() {
				opts.UseFrecency = true
			} else {
				return cmdutil.FlagErrorf("interactive mode required with no arguments")
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
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.UseFrecency {
		opts.IO.StartProgressIndicator()
		issues, err := opts.Frecency.GetFrecent(baseRepo, false)
		if err != nil {
			return err
		}
		opts.IO.StopProgressIndicator()

		selected, err := shared.SelectIssueNumber(issues)
		if err != nil {
			return err
		}
		opts.SelectorArg = selected
	}

	apiClient := api.NewClientFromHTTP(httpClient)

	issue, baseRepo, err := shared.IssueFromArg(apiClient, opts.BaseRepo, opts.SelectorArg)
	if err != nil {
		return err
	}

	if issue.State == "CLOSED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue #%d (%s) is already closed\n", cs.Yellow("!"), issue.Number, issue.Title)
		return nil
	}

	err = api.IssueClose(apiClient, baseRepo, *issue)
	if err != nil {
		return err
	}

	err = opts.Frecency.DeleteByNumber(baseRepo, false, issue.Number)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Closed issue #%d (%s)\n", cs.SuccessIconWithColor(cs.Red), issue.Number, issue.Title)

	return nil
}
