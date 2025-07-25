package view

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		tty      bool
		wants    ViewOptions
		wantsErr bool
	}{
		{
			name:     "blank nontty",
			wantsErr: true,
		},
		{
			name: "blank tty",
			tty:  true,
			wants: ViewOptions{
				Prompt: true,
			},
		},
		{
			name: "web tty",
			tty:  true,
			cli:  "--web",
			wants: ViewOptions{
				Prompt: true,
				Web:    true,
			},
		},
		{
			name: "web nontty",
			cli:  "1234 --web",
			wants: ViewOptions{
				Web:   true,
				RunID: "1234",
			},
		},
		{
			name:     "disallow web and log",
			tty:      true,
			cli:      "-w --log",
			wantsErr: true,
		},
		{
			name:     "disallow log and log-failed",
			tty:      true,
			cli:      "--log --log-failed",
			wantsErr: true,
		},
		{
			name: "exit status",
			cli:  "--exit-status 1234",
			wants: ViewOptions{
				RunID:      "1234",
				ExitStatus: true,
			},
		},
		{
			name: "verbosity",
			cli:  "-v",
			tty:  true,
			wants: ViewOptions{
				Verbose: true,
				Prompt:  true,
			},
		},
		{
			name: "with arg nontty",
			cli:  "1234",
			wants: ViewOptions{
				RunID: "1234",
			},
		},
		{
			name: "job id passed",
			cli:  "--job 1234",
			wants: ViewOptions{
				JobID: "1234",
			},
		},
		{
			name: "log passed",
			tty:  true,
			cli:  "--log",
			wants: ViewOptions{
				Prompt: true,
				Log:    true,
			},
		},
		{
			name: "tolerates both run and job id",
			cli:  "1234 --job 4567",
			wants: ViewOptions{
				JobID: "4567",
			},
		},
		{
			name: "run id with attempt",
			cli:  "1234 --attempt 2",
			wants: ViewOptions{
				RunID:   "1234",
				Attempt: 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					return config.NewBlankConfig(), nil
				},
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ViewOptions
			cmd := NewCmdView(f, func(opts *ViewOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.wants.RunID, gotOpts.RunID)
			assert.Equal(t, tt.wants.Prompt, gotOpts.Prompt)
			assert.Equal(t, tt.wants.ExitStatus, gotOpts.ExitStatus)
			assert.Equal(t, tt.wants.Verbose, gotOpts.Verbose)
			assert.Equal(t, tt.wants.Attempt, gotOpts.Attempt)
		})
	}
}

func TestViewRun(t *testing.T) {
	emptyZipArchive := createZipArchive(t, map[string]string{})
	zipArchive := createZipArchive(t, map[string]string{
		"0_cool job.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
			log line 1
			log line 2
			log line 3`),
		"cool job/1_fob the barz.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
		"cool job/2_barz the fob.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
		"1_sad job.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
			log line 1
			log line 2
			log line 3
		`),
		"sad job/1_barf the quux.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
		"sad job/2_quuz the barf.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
		"2_cool job with no step logs.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
		"3_sad job with no step logs.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
		"-9999999999_legacy cool job with no step logs.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
		"-9999999999_legacy sad job with no step logs.txt": heredoc.Doc(`
			log line 1
			log line 2
			log line 3
		`),
	})

	tests := []struct {
		name        string
		httpStubs   func(*httpmock.Registry)
		promptStubs func(*prompter.MockPrompter)
		opts        *ViewOptions
		tty         bool
		wantErr     bool
		wantOut     string
		browsedURL  string
		errMsg      string
	}{
		{
			name: "associate with PR",
			tty:  true,
			opts: &ViewOptions{
				RunID:  "3",
				Prompt: false,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(`{"data": {
		"repository": {
			"pullRequests": {
				"nodes": [
					{"number": 2898,
						"headRepository": {
						"owner": {
						"login": "OWNER"
						},
												"name": "REPO"}}
				]}}}}`))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
			},
			wantOut: "\n✓ trunk CI OWNER/REPO#2898 · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about the job, try: gh run view --job=10\nView this run on GitHub: https://github.com/runs/3\n",
		},
		{
			name: "associate with PR with attempt",
			tty:  true,
			opts: &ViewOptions{
				RunID:   "3",
				Attempt: 3,
				Prompt:  false,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(`{"data": {
		"repository": {
			"pullRequests": {
				"nodes": [
					{"number": 2898,
						"headRepository": {
						"owner": {
						"login": "OWNER"
						},
												"name": "REPO"}}
				]}}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
			},
			wantOut: "\n✓ trunk CI OWNER/REPO#2898 · 3 (Attempt #3)\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about the job, try: gh run view --job=10\nView this run on GitHub: https://github.com/runs/3/attempts/3\n",
		},
		{
			name: "exit status, failed run",
			opts: &ViewOptions{
				RunID:      "1234",
				ExitStatus: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/20/annotations"),
					httpmock.JSONResponse(shared.FailedJobAnnotations))
			},
			wantOut: "\nX trunk CI · 1234\nTriggered via push about 59 minutes ago\n\nJOBS\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nTo see what failed, try: gh run view 1234 --log-failed\nView this run on GitHub: https://github.com/runs/1234\n",
			wantErr: true,
		},
		{
			name: "with artifacts",
			opts: &ViewOptions{
				RunID: "3",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.JSONResponse(map[string][]shared.Artifact{
						"artifacts": {
							shared.Artifact{Name: "artifact-1", Expired: false},
							shared.Artifact{Name: "artifact-2", Expired: true},
							shared.Artifact{Name: "artifact-3", Expired: false},
						},
					}))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: heredoc.Doc(`

				✓ trunk CI · 3
				Triggered via push about 59 minutes ago

				JOBS


				ARTIFACTS
				artifact-1
				artifact-2 (expired)
				artifact-3

				For more information about a job, try: gh run view --job=<job-id>
				View this run on GitHub: https://github.com/runs/3
			`),
		},
		{
			name: "with artifacts and attempt",
			opts: &ViewOptions{
				RunID:   "3",
				Attempt: 3,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.JSONResponse(map[string][]shared.Artifact{
						"artifacts": {
							shared.Artifact{Name: "artifact-1", Expired: false},
							shared.Artifact{Name: "artifact-2", Expired: true},
							shared.Artifact{Name: "artifact-3", Expired: false},
						},
					}))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: heredoc.Doc(`

				✓ trunk CI · 3 (Attempt #3)
				Triggered via push about 59 minutes ago

				JOBS


				ARTIFACTS
				artifact-1
				artifact-2 (expired)
				artifact-3

				For more information about a job, try: gh run view --job=<job-id>
				View this run on GitHub: https://github.com/runs/3/attempts/3
			`),
		},
		{
			name: "exit status, successful run",
			opts: &ViewOptions{
				RunID:      "3",
				ExitStatus: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "\n✓ trunk CI · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about the job, try: gh run view --job=10\nView this run on GitHub: https://github.com/runs/3\n",
		},
		{
			name: "exit status, successful run, with attempt",
			opts: &ViewOptions{
				RunID:      "3",
				Attempt:    3,
				ExitStatus: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "\n✓ trunk CI · 3 (Attempt #3)\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about the job, try: gh run view --job=10\nView this run on GitHub: https://github.com/runs/3/attempts/3\n",
		},
		{
			name: "verbose",
			tty:  true,
			opts: &ViewOptions{
				RunID:   "1234",
				Prompt:  false,
				Verbose: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/20/annotations"),
					httpmock.JSONResponse(shared.FailedJobAnnotations))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "\nX trunk CI · 1234\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nTo see what failed, try: gh run view 1234 --log-failed\nView this run on GitHub: https://github.com/runs/1234\n",
		},
		{
			name: "prompts for choice, one job",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
			},
			opts: &ViewOptions{
				Prompt: true,
			},
			wantOut: "\n✓ trunk CI · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about the job, try: gh run view --job=10\nView this run on GitHub: https://github.com/runs/3\n",
		},
		{
			name: "interactive with log",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool job")
					})
			},
			wantOut: coolJobRunLogOutput,
		},
		{
			name: "interactive with log and attempt",
			tty:  true,
			opts: &ViewOptions{
				Prompt:  true,
				Attempt: 3,
				Log:     true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool job")
					})
			},
			wantOut: coolJobRunLogOutput,
		},
		{
			name: "noninteractive with log",
			opts: &ViewOptions{
				JobID: "10",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: coolJobRunLogOutput,
		},
		{
			name: "noninteractive with log and attempt",
			opts: &ViewOptions{
				JobID:   "10",
				Attempt: 3,
				Log:     true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/attempts/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: coolJobRunLogOutput,
		},
		{
			name: "interactive with run log",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: expectedRunLogOutput,
		},
		{
			name: "noninteractive with run log",
			tty:  true,
			opts: &ViewOptions{
				RunID: "3",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: expectedRunLogOutput,
		},
		{
			name: "interactive with log-failed",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "X sad job")
					})
			},
			wantOut: quuxTheBarfLogOutput,
		},
		{
			name: "interactive with log-failed with attempt",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				Attempt:   3,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/attempts/3"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/attempts/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/attempts/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "X sad job")
					})
			},
			wantOut: quuxTheBarfLogOutput,
		},
		{
			name: "noninteractive with log-failed",
			opts: &ViewOptions{
				JobID:     "20",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20"),
					httpmock.JSONResponse(shared.FailedJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: quuxTheBarfLogOutput,
		},
		{
			name: "interactive with run log-failed",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: quuxTheBarfLogOutput,
		},
		{
			name: "noninteractive with run log-failed",
			tty:  true,
			opts: &ViewOptions{
				RunID:     "1234",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: quuxTheBarfLogOutput,
		},
		{
			name: "interactive with log, with no step logs available (#10551)",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJobWithoutStepLogs,
							shared.FailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job with no step logs", "X sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool job with no step logs")
					})
			},
			wantOut: coolJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "noninteractive with log, with no step logs available (#10551)",
			opts: &ViewOptions{
				JobID: "11",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/11"),
					httpmock.JSONResponse(shared.SuccessfulJobWithoutStepLogs))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: coolJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "interactive with log-failed, with no step logs available (#10551)",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJobWithoutStepLogs,
							shared.FailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job with no step logs", "X sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "X sad job with no step logs")
					})
			},
			wantOut: sadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "noninteractive with log-failed, with no step logs available (#10551)",
			opts: &ViewOptions{
				JobID:     "21",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/21"),
					httpmock.JSONResponse(shared.FailedJobWithoutStepLogs))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: sadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "interactive with run log, with no step logs available (#10551)",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJobWithoutStepLogs,
							shared.FailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job with no step logs", "X sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: expectedRunLogOutputWithNoSteps,
		},
		{
			name: "noninteractive with run log, with no step logs available (#10551)",
			opts: &ViewOptions{
				RunID: "3",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJobWithoutStepLogs,
							shared.FailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: expectedRunLogOutputWithNoSteps,
		},
		{
			name: "interactive with run log-failed, with no step logs available (#10551)",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJobWithoutStepLogs,
							shared.FailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job with no step logs", "X sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: sadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "noninteractive with run log-failed, with no step logs available (#10551)",
			opts: &ViewOptions{
				RunID:     "1234",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJobWithoutStepLogs,
							shared.FailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: sadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "interactive with log, legacy service data, with no step logs available",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.LegacySuccessfulJobWithoutStepLogs,
							shared.LegacyFailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ legacy cool job with no step logs", "X legacy sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ legacy cool job with no step logs")
					})
			},
			wantOut: legacyCoolJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "noninteractive with log, legacy service data, with no step logs available",
			opts: &ViewOptions{
				JobID: "12",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/12"),
					httpmock.JSONResponse(shared.LegacySuccessfulJobWithoutStepLogs))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: legacyCoolJobRunWithNoStepLogsLogOutput,
		},

		{
			name: "interactive with log-failed, legacy service data, with no step logs available (#10551)",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.LegacySuccessfulJobWithoutStepLogs,
							shared.LegacyFailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ legacy cool job with no step logs", "X legacy sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "X legacy sad job with no step logs")
					})
			},
			wantOut: legacySadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "noninteractive with log-failed, legacy service data, with no step logs available (#10551)",
			opts: &ViewOptions{
				JobID:     "22",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/22"),
					httpmock.JSONResponse(shared.LegacyFailedJobWithoutStepLogs))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: legacySadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "interactive with run log, legacy service data, with no step logs available (#10551)",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.LegacySuccessfulJobWithoutStepLogs,
							shared.LegacyFailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ legacy cool job with no step logs", "X legacy sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: expectedLegacyRunLogOutputWithNoSteps,
		},
		{
			name: "noninteractive with run log, legacy service data, with no step logs available (#10551)",
			opts: &ViewOptions{
				RunID: "3",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.LegacySuccessfulJobWithoutStepLogs,
							shared.LegacyFailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: expectedLegacyRunLogOutputWithNoSteps,
		},
		{
			name: "interactive with run log-failed, legacy service data, with no step logs available (#10551)",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.LegacySuccessfulJobWithoutStepLogs,
							shared.LegacyFailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ legacy cool job with no step logs", "X legacy sad job with no step logs"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: legacySadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "noninteractive with run log-failed, legacy service data, with no step logs available (#10551)",
			opts: &ViewOptions{
				RunID:     "1234",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.LegacySuccessfulJobWithoutStepLogs,
							shared.LegacyFailedJobWithoutStepLogs,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(zipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: legacySadJobRunWithNoStepLogsLogOutput,
		},
		{
			name: "interactive with log, fallback to retrieve job logs from API (#11169)",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("blah blah"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool job")
					})
			},
			wantOut: "cool job\tUNKNOWN STEP\tblah blah\n",
		},
		{
			name: "noninteractive with log, fallback to retrieve job logs from API (#11169)",
			opts: &ViewOptions{
				JobID: "10",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("blah blah"))
			},
			wantOut: "cool job\tUNKNOWN STEP\tblah blah\n",
		},
		{
			name: "interactive with run log, fallback to retrieve job logs from API (#11169)",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("blah blah"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20/logs"),
					httpmock.StringResponse("yo yo"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: "cool job\tUNKNOWN STEP\tblah blah\nsad job\tUNKNOWN STEP\tyo yo\n",
		},
		{
			name: "noninteractive with run log, fallback to retrieve job logs from API (#11169)",
			opts: &ViewOptions{
				RunID: "3",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("blah blah"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20/logs"),
					httpmock.StringResponse("yo yo"))
			},
			wantOut: "cool job\tUNKNOWN STEP\tblah blah\nsad job\tUNKNOWN STEP\tyo yo\n",
		},
		{
			name: "interactive with log-failed, fallback to retrieve job logs from API (#11169)",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20/logs"),
					httpmock.StringResponse("yo yo"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "X sad job")
					})
			},
			wantOut: "sad job\tUNKNOWN STEP\tyo yo\n",
		},
		{
			name: "noninteractive with log-failed, fallback to retrieve job logs from API (#11169)",
			opts: &ViewOptions{
				JobID:     "20",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20"),
					httpmock.JSONResponse(shared.FailedJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20/logs"),
					httpmock.StringResponse("yo yo"))
			},
			wantOut: "sad job\tUNKNOWN STEP\tyo yo\n",
		},
		{
			name: "interactive with run log-failed, fallback to retrieve job logs from API (#11169)",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20/logs"),
					httpmock.StringResponse("yo yo"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: "sad job\tUNKNOWN STEP\tyo yo\n",
		},
		{
			name: "noninteractive with run log-failed, fallback to retrieve job logs from API (#11169)",
			opts: &ViewOptions{
				RunID:     "1234",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20/logs"),
					httpmock.StringResponse("yo yo"))
			},
			wantOut: "sad job\tUNKNOWN STEP\tyo yo\n",
		},
		{
			name: "interactive with run log, too many API calls required error (#11169)",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
				Log:    true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))

				tooManyJobs := make([]shared.Job, 1+maxAPILogFetchers)
				for i := range tooManyJobs {
					tooManyJobs[i] = shared.SuccessfulJob
					tooManyJobs[i].ID = int64(i + 100)
				}
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: tooManyJobs,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					slices.Concat([]string{"View all jobs in this run"}, slices.Repeat([]string{"✓ cool job"}, 26)),
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantErr: true,
			errMsg:  "too many API requests needed to fetch logs; try narrowing down to a specific job with the `--job` option",
		},
		{
			name: "noninteractive with run log, too many API calls required error (#11169)",
			opts: &ViewOptions{
				RunID: "3",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))

				tooManyJobs := make([]shared.Job, 1+maxAPILogFetchers)
				for i := range tooManyJobs {
					tooManyJobs[i] = shared.SuccessfulJob
					tooManyJobs[i].ID = int64(i + 100)
				}
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: tooManyJobs,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantErr: true,
			errMsg:  "too many API requests needed to fetch logs; try narrowing down to a specific job with the `--job` option",
		},
		{
			name: "interactive with run log-failed, too many API calls required error (#11169)",
			tty:  true,
			opts: &ViewOptions{
				Prompt:    true,
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))

				tooManyJobs := make([]shared.Job, 1+maxAPILogFetchers)
				for i := range tooManyJobs {
					tooManyJobs[i] = shared.FailedJob
					tooManyJobs[i].ID = int64(i + 100)
				}
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: tooManyJobs,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 4, nil
					})
				pm.RegisterSelect("View a specific job in this run?",
					slices.Concat([]string{"View all jobs in this run"}, slices.Repeat([]string{"X sad job"}, 26)),
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantErr: true,
			errMsg:  "too many API requests needed to fetch logs; try narrowing down to a specific job with the `--job` option",
		},
		{
			name: "noninteractive with run log-failed, too many API calls required error (#11169)",
			opts: &ViewOptions{
				RunID:     "1234",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))

				tooManyJobs := make([]shared.Job, 1+maxAPILogFetchers)
				for i := range tooManyJobs {
					tooManyJobs[i] = shared.FailedJob
					tooManyJobs[i].ID = int64(i + 100)
				}
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: tooManyJobs,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantErr: true,
			errMsg:  "too many API requests needed to fetch logs; try narrowing down to a specific job with the `--job` option",
		},
		{
			name: "noninteractive with run log-failed, maximum API calls allowed (#11169)",
			opts: &ViewOptions{
				RunID:     "1234",
				LogFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))

				tooManyJobs := make([]shared.Job, maxAPILogFetchers)
				for i := range tooManyJobs {
					tooManyJobs[i] = shared.FailedJob
					tooManyJobs[i].ID = int64(i + 100)
				}
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: tooManyJobs,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				for i := range tooManyJobs {
					reg.Register(
						httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/actions/jobs/%d/logs", i+100)),
						httpmock.StringResponse("yo yo"))
				}
			},
			wantOut: strings.Repeat("sad job\tUNKNOWN STEP\tyo yo\n", maxAPILogFetchers),
		},
		{
			name: "run log but run is not done",
			tty:  true,
			opts: &ViewOptions{
				RunID: "2",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
					httpmock.JSONResponse(shared.TestRun(2, shared.InProgress, "")))
				reg.Register(
					httpmock.REST("GET", "runs/2/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantErr: true,
			errMsg:  "run 2 is still in progress; logs will be available when it is complete",
		},
		{
			name: "job log but job is not done",
			tty:  true,
			opts: &ViewOptions{
				JobID: "20",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20"),
					httpmock.JSONResponse(shared.Job{
						ID:     20,
						Status: shared.InProgress,
						RunID:  2,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
					httpmock.JSONResponse(shared.TestRun(2, shared.InProgress, "")))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantErr: true,
			errMsg:  "job 20 is still in progress; logs will be available when it is complete",
		},
		{
			name: "job log but job is skipped",
			tty:  false,
			opts: &ViewOptions{
				JobID: "13",
				Log:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/13"),
					httpmock.JSONResponse(shared.SkippedJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/logs"),
					httpmock.BinaryResponse(emptyZipArchive))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "",
		},
		{
			name: "noninteractive with job",
			opts: &ViewOptions{
				JobID: "10",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "\n✓ trunk CI · 3\nTriggered via push about 59 minutes ago\n\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\nTo see the full job log, try: gh run view --log --job=10\nView this run on GitHub: https://github.com/runs/3\n",
		},
		{
			name: "interactive, multiple jobs, choose all jobs",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/20/annotations"),
					httpmock.JSONResponse(shared.FailedJobAnnotations))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "View all jobs in this run")
					})
			},
			wantOut: "\n✓ trunk CI · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nFor more information about a job, try: gh run view --job=<job-id>\nView this run on GitHub: https://github.com/runs/3\n",
		},
		{
			name: "interactive, multiple jobs, choose specific jobs",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "✓ cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "* cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool commit, CI [trunk] Feb 23, 2021")
					})
				pm.RegisterSelect("View a specific job in this run?",
					[]string{"View all jobs in this run", "✓ cool job", "X sad job"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "✓ cool job")
					})
			},
			wantOut: "\n✓ trunk CI · 3\nTriggered via push about 59 minutes ago\n\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\nTo see the full job log, try: gh run view --log --job=10\nView this run on GitHub: https://github.com/runs/3\n",
		},
		{
			name: "web run",
			tty:  true,
			opts: &ViewOptions{
				RunID: "3",
				Web:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			browsedURL: "https://github.com/runs/3",
			wantOut:    "Opening https://github.com/runs/3 in your browser.\n",
		},
		{
			name: "web job",
			tty:  true,
			opts: &ViewOptions{
				JobID: "10",
				Web:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			browsedURL: "https://github.com/jobs/10?check_suite_focus=true",
			wantOut:    "Opening https://github.com/jobs/10 in your browser.\n",
		},
		{
			name: "hide job header, failure",
			tty:  true,
			opts: &ViewOptions{
				RunID: "123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/123"),
					httpmock.JSONResponse(shared.TestRun(123, shared.Completed, shared.Failure)))
				reg.Register(
					httpmock.REST("GET", "runs/123/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{Jobs: []shared.Job{}}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/123/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "\nX trunk CI · 123\nTriggered via push about 59 minutes ago\n\nX This run likely failed because of a workflow file issue.\n\nFor more information, see: https://github.com/runs/123\n",
		},
		{
			name: "hide job header, startup_failure",
			tty:  true,
			opts: &ViewOptions{
				RunID: "123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/123"),
					httpmock.JSONResponse(shared.TestRun(123, shared.Completed, shared.StartupFailure)))
				reg.Register(
					httpmock.REST("GET", "runs/123/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{Jobs: []shared.Job{}}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/123/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "\nX trunk CI · 123\nTriggered via push about 59 minutes ago\n\nX This run likely failed because of a workflow file issue.\n\nFor more information, see: https://github.com/runs/123\n",
		},
		{
			name: "Fetches all of a run's jobs with --json flag",
			opts: &ViewOptions{
				RunID: "3",
				Exporter: shared.MakeTestExporter(
					[]string{"jobs"},
					func(io *iostreams.IOStreams, data interface{}) error {
						run, ok := data.(*shared.Run)
						if !ok {
							return fmt.Errorf("expected data type *shared.Run")
						}
						fmt.Fprintf(io.Out, "fetched %d jobs\n", len(run.Jobs))
						return nil
					},
				),
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.QueryMatcher("GET", "runs/3/jobs", url.Values{"per_page": []string{"100"}}),
					httpmock.WithHeader(
						httpmock.StringResponse(`{"jobs":[{},{},{}]}`),
						"Link",
						`<https://api.github.com/runs/3/jobs?page=2>; rel="next", <https://api.github.com/runs/3/jobs?page=2>; rel="last"`),
				)
				reg.Register(
					httpmock.REST("GET", "runs/3/jobs"),
					httpmock.StringResponse(`{"jobs":[{},{}]}`))
			},
			wantOut: "fetched 5 jobs\n",
		},
		{
			name: "Returns error when failing to get annotations",
			opts: &ViewOptions{
				RunID:      "1234",
				ExitStatus: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/20/annotations"),
					httpmock.StatusStringResponse(500, "internal server error"),
				)
			},
			errMsg:  "failed to get annotations: HTTP 500 (https://api.github.com/repos/OWNER/REPO/check-runs/20/annotations)",
			wantErr: true,
		},
		{
			name: "annotation endpoint forbidden (fine grained tokens)",
			tty:  true,
			opts: &ViewOptions{
				RunID: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234/artifacts"),
					httpmock.StringResponse(`{}`))
				reg.Register(
					httpmock.GraphQL(`query PullRequestForRun`),
					httpmock.StringResponse(``))
				reg.Register(
					httpmock.REST("GET", "runs/1234/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/20/annotations"),
					httpmock.StatusStringResponse(403, "Forbidden"))
			},
			wantOut: "\nX trunk CI · 1234\nTriggered via push about 59 minutes ago\n\nJOBS\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nrequesting annotations returned 403 Forbidden as the token does not have sufficient permissions. Note that it is not currently possible to create a fine-grained PAT with the `checks:read` permission.\n\nTo see what failed, try: gh run view 1234 --log-failed\nView this run on GitHub: https://github.com/runs/1234\n",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.Now = func() time.Time {
			notnow, _ := time.Parse("2006-01-02 15:04:05", "2021-02-23 05:50:00")
			return notnow
		}

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("OWNER/REPO")
		}

		pm := prompter.NewMockPrompter(t)
		tt.opts.Prompter = pm
		if tt.promptStubs != nil {
			tt.promptStubs(pm)
		}

		browser := &browser.Stub{}
		tt.opts.Browser = browser
		tt.opts.RunLogCache = RunLogCache{
			cacheDir: t.TempDir(),
		}

		t.Run(tt.name, func(t *testing.T) {
			err := runView(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Equal(t, tt.errMsg, err.Error())
				}
				if !tt.opts.ExitStatus {
					return
				}
			}
			if !tt.opts.ExitStatus {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			if tt.browsedURL != "" {
				assert.Equal(t, tt.browsedURL, browser.BrowsedURL())
			}
			reg.Verify(t)
		})
	}
}

var barfTheFobLogOutput = heredoc.Doc(`
cool job	barz the fob	log line 1
cool job	barz the fob	log line 2
cool job	barz the fob	log line 3
`)

var fobTheBarzLogOutput = heredoc.Doc(`
cool job	fob the barz	log line 1
cool job	fob the barz	log line 2
cool job	fob the barz	log line 3
`)

var barfTheQuuxLogOutput = heredoc.Doc(`
sad job	barf the quux	log line 1
sad job	barf the quux	log line 2
sad job	barf the quux	log line 3
`)

var quuxTheBarfLogOutput = heredoc.Doc(`
sad job	quux the barf	log line 1
sad job	quux the barf	log line 2
sad job	quux the barf	log line 3
`)

var coolJobRunWithNoStepLogsLogOutput = heredoc.Doc(`
cool job with no step logs	UNKNOWN STEP	log line 1
cool job with no step logs	UNKNOWN STEP	log line 2
cool job with no step logs	UNKNOWN STEP	log line 3
`)

var legacyCoolJobRunWithNoStepLogsLogOutput = heredoc.Doc(`
legacy cool job with no step logs	UNKNOWN STEP	log line 1
legacy cool job with no step logs	UNKNOWN STEP	log line 2
legacy cool job with no step logs	UNKNOWN STEP	log line 3
`)

var sadJobRunWithNoStepLogsLogOutput = heredoc.Doc(`
sad job with no step logs	UNKNOWN STEP	log line 1
sad job with no step logs	UNKNOWN STEP	log line 2
sad job with no step logs	UNKNOWN STEP	log line 3
`)

var legacySadJobRunWithNoStepLogsLogOutput = heredoc.Doc(`
legacy sad job with no step logs	UNKNOWN STEP	log line 1
legacy sad job with no step logs	UNKNOWN STEP	log line 2
legacy sad job with no step logs	UNKNOWN STEP	log line 3
`)

var coolJobRunLogOutput = fmt.Sprintf("%s%s", fobTheBarzLogOutput, barfTheFobLogOutput)
var sadJobRunLogOutput = fmt.Sprintf("%s%s", barfTheQuuxLogOutput, quuxTheBarfLogOutput)
var expectedRunLogOutput = fmt.Sprintf("%s%s", coolJobRunLogOutput, sadJobRunLogOutput)
var expectedRunLogOutputWithNoSteps = fmt.Sprintf("%s%s", coolJobRunWithNoStepLogsLogOutput, sadJobRunWithNoStepLogsLogOutput)
var expectedLegacyRunLogOutputWithNoSteps = fmt.Sprintf("%s%s", legacyCoolJobRunWithNoStepLogsLogOutput, legacySadJobRunWithNoStepLogsLogOutput)

func TestRunLog(t *testing.T) {
	t.Run("when the cache dir doesn't exist, exists return false", func(t *testing.T) {
		cacheDir := t.TempDir() + "/non-existent-dir"
		rlc := RunLogCache{cacheDir: cacheDir}

		exists, err := rlc.Exists("unimportant-key")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("when no cache entry has been created, exists returns false", func(t *testing.T) {
		cacheDir := t.TempDir()
		rlc := RunLogCache{cacheDir: cacheDir}

		exists, err := rlc.Exists("unimportant-key")
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("when a cache entry has been created, exists returns true", func(t *testing.T) {
		cacheDir := t.TempDir()
		rlc := RunLogCache{cacheDir: cacheDir}

		contents := strings.NewReader("unimportant-content")
		require.NoError(t, rlc.Create("key", contents))

		exists, err := rlc.Exists("key")
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("when the cache dir doesn't exist, creating a cache entry creates it", func(t *testing.T) {
		cacheDir := t.TempDir() + "/non-existent-dir"
		rlc := RunLogCache{cacheDir: cacheDir}

		contents := strings.NewReader("unimportant-content")
		require.NoError(t, rlc.Create("key", contents))

		require.DirExists(t, cacheDir)
	})

	t.Run("when a cache entry has been created, reading it returns its contents", func(t *testing.T) {
		cacheDir := t.TempDir()
		rlc := RunLogCache{cacheDir: cacheDir}

		raw := createZipArchive(t, map[string]string{"foo": "bar"})
		f := bytes.NewReader(raw)

		require.NoError(t, rlc.Create("key", f))

		zipReader, err := rlc.Open("key")
		require.NoError(t, err)
		defer zipReader.Close()
		require.NotEmpty(t, zipReader.File)
		require.Equal(t, "foo", zipReader.File[0].Name)
	})
}
