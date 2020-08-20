package checks

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ChecksOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
	Remotes    func() (context.Remotes, error)

	SelectorArg string
}

func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	opts := &ChecksOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Branch:     f.Branch,
		Remotes:    f.Remotes,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "checks",
		Short: "Show CI status for a single pull request",
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

			return checksRun(opts)
		},
	}

	return cmd
}

func checksRun(opts *ChecksOptions) error {
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

	runList, err := checkRuns(apiClient, repo, pr)
	if err != nil {
		return err
	}

	if len(runList.CheckRuns) == 0 {
		return nil
	}

	tp := utils.NewTablePrinter(opts.IO)

	for _, cr := range runList.CheckRuns {
		var mark string
		switch cr.Status {
		case "pending":
			mark = utils.YellowDot()
		case "pass":
			mark = utils.GreenCheck()
		case "fail":
			mark = utils.RedX()
		}

		elapsed := fmt.Sprintf("%s", cr.Elapsed)
		if cr.Elapsed < 0 {
			elapsed = "0"
		}

		if opts.IO.IsStdoutTTY() {
			tp.AddField(mark, nil, nil)
			tp.AddField(cr.Name, nil, nil)
			tp.AddField(elapsed, nil, nil)
			tp.AddField(cr.Link, nil, nil)
		} else {
			tp.AddField(cr.Name, nil, nil)
			tp.AddField(cr.Status, nil, nil)
			tp.AddField(elapsed, nil, nil)
			tp.AddField(cr.Link, nil, nil)
		}

		tp.EndRow()
	}

	fmt.Fprintln(opts.IO.Out, runList.Summary())
	fmt.Fprintln(opts.IO.Out)

	return tp.Render()
}

type checkRun struct {
	Name    string
	Status  string
	Link    string
	Elapsed time.Duration
}

type checkRunList struct {
	Passing   int
	Failing   int
	Pending   int
	CheckRuns []checkRun
}

func (runList *checkRunList) Summary() string {
	fails := runList.Failing
	passes := runList.Passing
	pending := runList.Pending

	if fails+passes+pending == 0 {
		return ""
	}

	summary := ""

	if runList.Failing > 0 {
		summary = "Some checks were not successful"
	} else if runList.Pending > 0 {
		summary = "Some checks are still pending"
	} else {
		summary = "All checks were successful"
	}

	tallies := fmt.Sprintf(
		"%d failing, %d successful, and %d pending checks",
		fails, passes, pending)

	return fmt.Sprintf("%s\n%s", utils.Bold(summary), tallies)
}
