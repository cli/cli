package watch

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdWatch(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		tty      bool
		wants    WatchOptions
		wantsErr bool
	}{
		{
			name:     "blank nontty",
			wantsErr: true,
		},
		{
			name: "blank tty",
			tty:  true,
			wants: WatchOptions{
				Prompt:   true,
				Interval: defaultInterval,
			},
		},
		{
			name: "interval",
			tty:  true,
			cli:  "-i10",
			wants: WatchOptions{
				Interval: 10,
				Prompt:   true,
			},
		},
		{
			name: "exit status",
			cli:  "1234 --exit-status",
			wants: WatchOptions{
				Interval:   defaultInterval,
				RunID:      "1234",
				ExitStatus: true,
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
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *WatchOptions
			cmd := NewCmdWatch(f, func(opts *WatchOptions) error {
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
			assert.Equal(t, tt.wants.Interval, gotOpts.Interval)
		})
	}
}

func TestWatchRun(t *testing.T) {
	failedRunStubs := func(reg *httpmock.Registry) {
		inProgressRun := shared.TestRunWithCommit(2, shared.InProgress, "", "commit2")
		completedRun := shared.TestRun(2, shared.Completed, shared.Failure)
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
			httpmock.JSONResponse(shared.RunsPayload{
				WorkflowRuns: []shared.Run{
					shared.TestRunWithCommit(1, shared.InProgress, "", "commit1"),
					inProgressRun,
				},
			}))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
			httpmock.JSONResponse(inProgressRun))
		reg.Register(
			httpmock.REST("GET", "runs/2/jobs"),
			httpmock.JSONResponse(shared.JobsPayload{
				Jobs: []shared.Job{}}))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
			httpmock.JSONResponse(completedRun))
		reg.Register(
			httpmock.REST("GET", "runs/2/jobs"),
			httpmock.JSONResponse(shared.JobsPayload{
				Jobs: []shared.Job{
					shared.FailedJob,
				},
			}))
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
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
			httpmock.JSONResponse(shared.TestWorkflow))
	}
	successfulRunStubs := func(reg *httpmock.Registry) {
		inProgressRun := shared.TestRunWithCommit(2, shared.InProgress, "", "commit2")
		completedRun := shared.TestRun(2, shared.Completed, shared.Success)
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
			httpmock.JSONResponse(shared.RunsPayload{
				WorkflowRuns: []shared.Run{
					shared.TestRunWithCommit(1, shared.InProgress, "", "commit1"),
					inProgressRun,
				},
			}))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
			httpmock.JSONResponse(inProgressRun))
		reg.Register(
			httpmock.REST("GET", "runs/2/jobs"),
			httpmock.JSONResponse(shared.JobsPayload{
				Jobs: []shared.Job{
					shared.SuccessfulJob,
				},
			}))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
			httpmock.JSONResponse([]shared.Annotation{}))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
			httpmock.JSONResponse(completedRun))
		reg.Register(
			httpmock.REST("GET", "runs/2/jobs"),
			httpmock.JSONResponse(shared.JobsPayload{
				Jobs: []shared.Job{
					shared.SuccessfulJob,
				},
			}))
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
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
			httpmock.JSONResponse(shared.TestWorkflow))
	}

	tests := []struct {
		name        string
		httpStubs   func(*httpmock.Registry)
		promptStubs func(*prompter.MockPrompter)
		opts        *WatchOptions
		tty         bool
		wantErr     bool
		errMsg      string
		wantOut     string
	}{
		{
			name: "run ID provided run already completed",
			opts: &WatchOptions{
				RunID: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "Run CI (1234) has already completed with 'failure'\n",
		},
		{
			name: "already completed, exit status",
			opts: &WatchOptions{
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
			},
			wantOut: "Run CI (1234) has already completed with 'failure'\n",
			wantErr: true,
			errMsg:  "SilentError",
		},
		{
			name: "prompt, no in progress runs",
			tty:  true,
			opts: &WatchOptions{
				Prompt: true,
			},
			wantErr: true,
			errMsg:  "found no in progress runs to watch",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: []shared.Run{
							shared.FailedRun,
							shared.SuccessfulRun,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
		},
		{
			name: "interval respected",
			tty:  true,
			opts: &WatchOptions{
				Interval: 0,
				Prompt:   true,
			},
			httpStubs: successfulRunStubs,
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"* commit1, CI [trunk] Feb 23, 2021", "* commit2, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "* commit2, CI [trunk] Feb 23, 2021")
					})
			},
			wantOut: "\x1b[?1049h\x1b[0;0H\x1b[JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n* trunk CI · 2\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\x1b[?1049l✓ trunk CI · 2\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\n✓ Run CI (2) completed with 'success'\n",
		},
		{
			name: "exit status respected",
			tty:  true,
			opts: &WatchOptions{
				Interval:   0,
				Prompt:     true,
				ExitStatus: true,
			},
			httpStubs: failedRunStubs,
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"* commit1, CI [trunk] Feb 23, 2021", "* commit2, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "* commit2, CI [trunk] Feb 23, 2021")
					})
			},
			wantOut: "\x1b[?1049h\x1b[0;0H\x1b[JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n* trunk CI · 2\nTriggered via push about 59 minutes ago\n\n\x1b[?1049lX trunk CI · 2\nTriggered via push about 59 minutes ago\n\nJOBS\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nX Run CI (2) completed with 'failure'\n",
			wantErr: true,
			errMsg:  "SilentError",
		},
		{
			name: "failed to get run status",
			tty:  true,
			opts: &WatchOptions{
				RunID: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.TestRunWithCommit(1234, shared.InProgress, "", "commit2")),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.StatusJSONResponse(404, api.HTTPError{
						StatusCode: 404,
						Message:    "run 1234 not found",
					}),
				)
			},
			wantOut: "\x1b[?1049h\x1b[?1049l",
			wantErr: true,
			errMsg:  "failed to get run: HTTP 404: run 1234 not found (https://api.github.com/repos/OWNER/REPO/actions/runs/1234?exclude_pull_requests=true)",
		},
		{
			name: "annotation endpoint forbidden (fine grained tokens)",
			tty:  true,
			opts: &WatchOptions{
				RunID: "2",
			},
			httpStubs: func(reg *httpmock.Registry) {
				inProgressRun := shared.TestRunWithCommit(2, shared.InProgress, "", "commit2")
				completedRun := shared.TestRun(2, shared.Completed, shared.Success)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
					httpmock.JSONResponse(inProgressRun))
				reg.Register(
					httpmock.REST("GET", "runs/2/jobs"),
					httpmock.JSONResponse(shared.JobsPayload{
						Jobs: []shared.Job{
							shared.SuccessfulJob,
							shared.FailedJob,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse(shared.SuccessfulJobAnnotations))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/20/annotations"),
					httpmock.StatusStringResponse(403, "Forbidden"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/2"),
					httpmock.JSONResponse(completedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(shared.TestWorkflow))
			},
			wantOut: "\x1b[?1049h\x1b[?1049l✓ trunk CI · 2\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nrequesting annotations returned 403 Forbidden as the token does not have sufficient permissions. Note that it is not currently possible to create a fine-grained PAT with the `checks:read` permission.\n✓ Run CI (2) completed with 'success'\n",
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
		ios.SetAlternateScreenBufferEnabled(tt.tty)
		tt.opts.IO = ios
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("OWNER/REPO")
		}

		t.Run(tt.name, func(t *testing.T) {
			pm := prompter.NewMockPrompter(t)
			tt.opts.Prompter = pm
			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

			err := watchRun(tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
			// avoiding using `assert.Equal` here because it would print raw escape sequences to stdout
			if got := stdout.String(); got != tt.wantOut {
				t.Errorf("got stdout:\n%q\nwant:\n%q", got, tt.wantOut)
			}
			reg.Verify(t)
		})
	}
}
