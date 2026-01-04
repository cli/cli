package create

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/capi"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const defaultLogPollInterval = 5 * time.Second

// CreateOptions holds options for create command
type CreateOptions struct {
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	CapiClient func() (capi.CapiClient, error)
	Config     func() (gh.Config, error)

	LogRenderer func() shared.LogRenderer
	Sleep       func(d time.Duration)

	ProblemStatement     string
	CustomAgent          string
	BackOff              backoff.BackOff
	BaseBranch           string
	Prompter             prompter.Prompter
	ProblemStatementFile string
	Follow               bool
}

func defaultLogRenderer() shared.LogRenderer {
	return shared.NewLogRenderer()
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:          f.IOStreams,
		CapiClient:  shared.CapiClientFunc(f),
		Config:      f.Config,
		Prompter:    f.Prompter,
		LogRenderer: defaultLogRenderer,
		Sleep:       time.Sleep,
	}

	cmd := &cobra.Command{
		Use:   "create [<task description>] [flags]",
		Short: "Create an agent task (preview)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Support -R/--repo override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("only one of -F or arg can be provided", len(args) > 0, opts.ProblemStatementFile != ""); err != nil {
				return err
			}

			// Populate ProblemStatement from arg
			if len(args) > 0 {
				opts.ProblemStatement = args[0]
				if strings.TrimSpace(opts.ProblemStatement) == "" {
					return cmdutil.FlagErrorf("task description cannot be empty")
				}
			} else if opts.ProblemStatementFile == "" && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("a task description or -F is required when running non-interactively")
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
		Example: heredoc.Doc(`
			# Create a task from an inline description
			$ gh agent-task create "build me a new app"

			# Create a task from an inline description and follow logs
			$ gh agent-task create "build me a new app" --follow

			# Create a task from a file
			$ gh agent-task create -F task-desc.md

			# Create a task with problem statement from stdin
			$ echo "build me a new app" | gh agent-task create -F -

			# Create a task with an editor
			$ gh agent-task create

			# Create a task with an editor and a file as a template
			$ gh agent-task create -F task-desc.md

			# Select a different base branch for the PR
			$ gh agent-task create "fix errors" --base branch

			# Create a task using the custom agent defined in '.github/agents/my-agent.md'
			$ gh agent-task create "build me a new app" --custom-agent my-agent
		`),
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.Flags().StringVarP(&opts.ProblemStatementFile, "from-file", "F", "", "Read task description from `file` (use \"-\" to read from standard input)")
	cmd.Flags().StringVarP(&opts.BaseBranch, "base", "b", "", "Base branch for the pull request (use default branch if not provided)")
	cmd.Flags().BoolVar(&opts.Follow, "follow", false, "Follow agent session logs")
	cmd.Flags().StringVarP(&opts.CustomAgent, "custom-agent", "a", "", "Use a custom agent for the task. e.g., use 'my-agent' for the 'my-agent.md' agent")

	return cmd
}

func createRun(opts *CreateOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil || repo == nil {
		// Not printing the error that came back from BaseRepo() here because we want
		// something clear, human friendly, and actionable.
		return fmt.Errorf("a repository is required; re-run in a repository or supply one with --repo owner/name")
	}

	if opts.ProblemStatement == "" {
		if opts.ProblemStatementFile != "" {
			fileContent, err := cmdutil.ReadFile(opts.ProblemStatementFile, opts.IO.In)
			if err != nil {
				return fmt.Errorf("could not read task description file: %w", err)
			}

			trimmed := strings.TrimSpace(string(fileContent))
			if trimmed == "" {
				return errors.New("task description file cannot be empty")
			}

			opts.ProblemStatement = trimmed
		} else {
			desc, err := opts.Prompter.MarkdownEditor("Enter the task description", opts.ProblemStatement, false)
			if err != nil {
				return err
			}

			trimmed := strings.TrimSpace(string(desc))
			if trimmed == "" {
				return errors.New("a task description is required")
			}

			opts.ProblemStatement = trimmed
		}
	}

	client, err := opts.CapiClient()
	if err != nil {
		return err
	}

	ctx := context.Background()
	opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Creating agent task in %s/%s...", repo.RepoOwner(), repo.RepoName()))
	defer opts.IO.StopProgressIndicator()

	job, err := client.CreateJob(ctx, repo.RepoOwner(), repo.RepoName(), opts.ProblemStatement, opts.BaseBranch, opts.CustomAgent)
	if err != nil {
		return err
	}

	if opts.Follow {
		opts.IO.StopProgressIndicator()
		fmt.Fprintf(opts.IO.Out, "Displaying session logs for job %s. Press Ctrl+C to stop.\n", job.ID)
		return followLogs(opts, client, job.SessionID)
	}

	sessionURL, err := fetchJobSessionURL(ctx, client, repo, job, opts.BackOff)
	opts.IO.StopProgressIndicator()

	if sessionURL != "" {
		fmt.Fprintln(opts.IO.Out, sessionURL)
	} else {
		if err != nil {
			// If this does happen ever, we still want the user to get the fallback
			// message and URL. So, we don't return with this error, but we do still
			// want to print it.
			fmt.Fprintf(opts.IO.ErrOut, "%v\n", err)
		}
		fmt.Fprintf(opts.IO.Out, "job %s queued. View progress: %s\n", job.ID, capi.AgentsHomeURL)
	}

	return nil
}

func agentSessionWebURL(repo ghrepo.Interface, j *capi.Job) string {
	if j.PullRequest == nil {
		return ""
	}
	if j.SessionID == "" {
		return fmt.Sprintf("https://github.com/%s/%s/pull/%d", url.PathEscape(repo.RepoOwner()), url.PathEscape(repo.RepoName()), j.PullRequest.Number)
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d/agent-sessions/%s", url.PathEscape(repo.RepoOwner()), url.PathEscape(repo.RepoName()), j.PullRequest.Number, url.PathEscape(j.SessionID))
}

// fetchJobSessionURL tries to return the agent session URL for a job. If the pull
// request is not yet available, ("", nil) is returned.
func fetchJobSessionURL(ctx context.Context, client capi.CapiClient, repo ghrepo.Interface, job *capi.Job, bo backoff.BackOff) (string, error) {
	if job.PullRequest != nil && job.PullRequest.Number > 0 {
		// Return the agent session URL if we happen to get it.
		// Right now, this never happens.
		return agentSessionWebURL(repo, job), nil
	}

	if bo == nil {
		bo = backoff.NewExponentialBackOff(
			backoff.WithMaxElapsedTime(10*time.Second),
			backoff.WithInitialInterval(300*time.Millisecond),
			backoff.WithMaxInterval(10*time.Second),
			backoff.WithMultiplier(1.5),
		)
	}

	jobWithPR, err := fetchJobWithBackoff(ctx, client, repo, job.ID, bo)
	if jobWithPR != nil {
		return agentSessionWebURL(repo, jobWithPR), nil
	}
	return "", err
}

// fetchJobWithBackoff polls the job resource until a PR number is present or the overall
// timeout elapses. It returns the updated Job on success, (nil, nil) on timeout,
// and (nil, error) only for non-retryable failures.
func fetchJobWithBackoff(ctx context.Context, client capi.CapiClient, repo ghrepo.Interface, jobID string, bo backoff.BackOff) (*capi.Job, error) {
	// sentinel error to signal timeout
	var errPRNotReady = errors.New("job not ready")

	var result *capi.Job
	retryErr := backoff.Retry(func() error {
		j, err := client.GetJob(ctx, repo.RepoOwner(), repo.RepoName(), jobID)
		if err != nil {
			// Do not retry on GetJob errors; surface immediately.
			return backoff.Permanent(err)
		}
		if j.PullRequest != nil && j.PullRequest.Number > 0 {
			result = j
			return nil
		}
		return errPRNotReady
	}, backoff.WithContext(bo, ctx))

	if retryErr != nil {
		if errors.Is(retryErr, errPRNotReady) {
			// Timed out
			return nil, nil
		}
		return nil, retryErr
	}
	return result, nil
}

func followLogs(opts *CreateOptions, capiClient capi.CapiClient, sessionID string) error {
	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}

	ctx := context.Background()
	renderer := opts.LogRenderer()

	var called bool
	fetcher := func() ([]byte, error) {
		if called {
			opts.Sleep(defaultLogPollInterval)
		}
		called = true
		raw, err := capiClient.GetSessionLogs(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		return raw, nil
	}

	return renderer.Follow(fetcher, opts.IO.Out, opts.IO)
}
