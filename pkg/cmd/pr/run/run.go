package run

import (
	"errors"
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

	oid := ""
	if len(pr.Commits.Nodes) > 0 {
		oid = pr.Commits.Nodes[0].Commit.Oid
	}

	if oid == "" {
		return fmt.Errorf("no commits could be found for pull request %d", pr.Number)
	}

	runList, err := shared.CheckRuns(apiClient, repo, pr)
	if err != nil || len(runList.CheckRuns) == 0 {
		return fmt.Errorf("could not find any runs for oid %s: %w", oid, err)
	}

	// now we have a run list. need to figure out which ones to request a rerun for.

	runner := &Runner{oid, opts, runList}

	switch {
	case opts.Failed:
		return runner.Failed()
	case opts.All:
		return runner.All()
	case opts.Check != "":
		return runner.Check()
	}

	// TODO Interactive multi-select of checks

	return nil
}

type Runner struct {
	Oid  string
	Opts *RunOptions
	Runs *shared.CheckRunList
}

func (r *Runner) All() error {
	return r.run(r.Runs.CheckRuns)
}

func (r *Runner) Failed() error {
	toRun := []shared.CheckRun{}
	for _, checkRun := range r.Runs.CheckRuns {
		if checkRun.Status == "fail" {
			toRun = append(toRun, checkRun)
		}
	}
	return r.run(toRun)
}

func (r *Runner) Check() error {
	checkName := r.Opts.Check
	if checkName == "" {
		return errors.New("check name can't be blank")
	}
	toRun := []shared.CheckRun{
		{
			Name: checkName,
		},
	}

	return r.run(toRun)
}

func (r *Runner) run(checkRuns []shared.CheckRun) error {
	// TODO

	for _, r := range checkRuns {
		fmt.Printf("DEBUG %#v\n", r)

	}
	return nil
}
