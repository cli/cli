package view

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	RunID      string
	Verbose    bool
	ExitStatus bool

	Prompt bool

	Now func() time.Time
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}
	cmd := &cobra.Command{
		Use:    "view [<run-id>]",
		Short:  "View a summary of a workflow run",
		Args:   cobra.MaximumNArgs(1),
		Hidden: true,
		Example: heredoc.Doc(`
		  # Interactively select a run to view
		  $ gh run view

		  # View a specific run
		  $ gh run view 0451

		  # Exit non-zero if a run failed
		  $ gh run view 0451 -e && echo "job pending or passed"
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.RunID = args[0]
			} else if !opts.IO.CanPrompt() {
				return &cmdutil.FlagError{Err: errors.New("run ID required when not running interactively")}
			} else {
				opts.Prompt = true
			}

			if runF != nil {
				return runF(opts)
			}
			return runView(opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "Show job steps")
	// TODO should we try and expose pending via another exit code?
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "Exit with non-zero status if run failed")

	return cmd
}

func runView(opts *ViewOptions) error {
	c, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}
	client := api.NewClientFromHTTP(c)

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("failed to determine base repo: %w", err)
	}

	runID := opts.RunID

	if opts.Prompt {
		cs := opts.IO.ColorScheme()
		runID, err = shared.PromptForRun(cs, client, repo)
		if err != nil {
			return err
		}
	}

	opts.IO.StartProgressIndicator()
	defer opts.IO.StopProgressIndicator()

	run, err := shared.GetRun(client, repo, runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	prNumber := ""
	number, err := prForRun(client, repo, *run)
	if err == nil {
		prNumber = fmt.Sprintf(" #%d", number)
	}

	jobs, err := shared.GetJobs(client, repo, *run)
	if err != nil {
		return fmt.Errorf("failed to get jobs: %w", err)
	}

	var annotations []shared.Annotation

	var annotationErr error
	var as []shared.Annotation
	for _, job := range jobs {
		as, annotationErr = shared.GetAnnotations(client, repo, job)
		if annotationErr != nil {
			break
		}
		annotations = append(annotations, as...)
	}

	opts.IO.StopProgressIndicator()
	if annotationErr != nil {
		return fmt.Errorf("failed to get annotations: %w", annotationErr)
	}

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	title := fmt.Sprintf("%s %s%s",
		cs.Bold(run.HeadBranch), run.Name, prNumber)
	symbol, symbolColor := shared.Symbol(cs, run.Status, run.Conclusion)
	id := cs.Cyanf("%d", run.ID)

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s · %s\n", symbolColor(symbol), title, id)

	ago := opts.Now().Sub(run.CreatedAt)

	fmt.Fprintf(out, "Triggered via %s %s\n", run.Event, utils.FuzzyAgo(ago))
	fmt.Fprintln(out)

	if len(jobs) == 0 && run.Conclusion == shared.Failure {
		fmt.Fprintf(out, "%s %s\n",
			cs.FailureIcon(),
			cs.Bold("This run likely failed because of a workflow file issue."))

		fmt.Fprintln(out)
		fmt.Fprintf(out, "For more information, see: %s\n", cs.Bold(run.URL))

		if opts.ExitStatus {
			return cmdutil.SilentError
		}
		return nil
	}

	fmt.Fprintln(out, cs.Bold("JOBS"))

	for _, job := range jobs {
		symbol, symbolColor := shared.Symbol(cs, job.Status, job.Conclusion)
		id := cs.Cyanf("%d", job.ID)
		fmt.Fprintf(out, "%s %s (ID %s)\n", symbolColor(symbol), job.Name, id)
		if opts.Verbose || shared.IsFailureState(job.Conclusion) {
			for _, step := range job.Steps {
				stepSymbol, stepSymColor := shared.Symbol(cs, step.Status, step.Conclusion)
				fmt.Fprintf(out, "  %s %s\n", stepSymColor(stepSymbol), step.Name)
			}
		}
	}

	if len(annotations) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, cs.Bold("ANNOTATIONS"))

		for _, a := range annotations {
			fmt.Fprintf(out, "%s %s\n", shared.AnnotationSymbol(cs, a), a.Message)
			fmt.Fprintln(out, cs.Grayf("%s: %s#%d\n",
				a.JobName, a.Path, a.StartLine))
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "For more information about a job, try: gh job view <job-id>")
	fmt.Fprintf(out, cs.Gray("view this run on GitHub: %s\n"), run.URL)

	if opts.ExitStatus && shared.IsFailureState(run.Conclusion) {
		return cmdutil.SilentError
	}

	return nil
}

func prForRun(client *api.Client, repo ghrepo.Interface, run shared.Run) (int, error) {
	type response struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number         int
					HeadRepository struct {
						Owner struct {
							Login string
						}
						Name string
					}
				}
			}
		}
		Number int
	}

	variables := map[string]interface{}{
		"owner":       repo.RepoOwner(),
		"repo":        repo.RepoName(),
		"headRefName": run.HeadBranch,
	}

	query := `
		query PullRequestForRun($owner: String!, $repo: String!, $headRefName: String!) {
			repository(owner: $owner, name: $repo) {
				pullRequests(headRefName: $headRefName, first: 1, orderBy: { field: CREATED_AT, direction: DESC }) {
					nodes {
						number
						headRepository {
							owner {
								login
							}
							name
						}
					}
				}
			}
		}`

	var resp response

	err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
	if err != nil {
		return -1, err
	}

	prs := resp.Repository.PullRequests.Nodes
	if len(prs) == 0 {
		return -1, fmt.Errorf("no matching PR found for %s", run.HeadBranch)
	}

	number := -1

	for _, pr := range prs {
		if pr.HeadRepository.Owner.Login == run.HeadRepository.Owner.Login && pr.HeadRepository.Name == run.HeadRepository.Name {
			number = pr.Number
		}
	}

	if number == -1 {
		return number, fmt.Errorf("no matching PR found for %s", run.HeadBranch)
	}

	return number, nil
}
