package watch

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
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
			io, _, _, _ := iostreams.Test()
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: io,
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
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)

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
		inProgressRun := shared.TestRun("more runs", 2, shared.InProgress, "")
		completedRun := shared.TestRun("more runs", 2, shared.Completed, shared.Failure)
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
			httpmock.JSONResponse(shared.RunsPayload{
				WorkflowRuns: []shared.Run{
					shared.TestRun("run", 1, shared.InProgress, ""),
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
	}
	successfulRunStubs := func(reg *httpmock.Registry) {
		inProgressRun := shared.TestRun("more runs", 2, shared.InProgress, "")
		completedRun := shared.TestRun("more runs", 2, shared.Completed, shared.Success)
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
			httpmock.JSONResponse(shared.RunsPayload{
				WorkflowRuns: []shared.Run{
					shared.TestRun("run", 1, shared.InProgress, ""),
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
	}

	tests := []struct {
		name        string
		httpStubs   func(*httpmock.Registry)
		askStubs    func(*prompt.AskStubber)
		opts        *WatchOptions
		tty         bool
		wantErr     bool
		errMsg      string
		wantOut     string
		onlyWindows bool
		skipWindows bool
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
			},
			wantOut: "Run failed (1234) has already completed with 'failure'\n",
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
			},
			wantOut: "Run failed (1234) has already completed with 'failure'\n",
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
			},
		},
		{
			name:        "interval respected",
			skipWindows: true,
			tty:         true,
			opts: &WatchOptions{
				Interval: 0,
				Prompt:   true,
			},
			httpStubs: successfulRunStubs,
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)
			},
			wantOut: "\x1b[2J\x1b[0;0H\x1b[JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n- trunk more runs · 2\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\x1b[0;0H\x1b[JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n✓ trunk more runs · 2\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\n✓ Run more runs (2) completed with 'success'\n",
		},
		{
			name:        "interval respected, windows",
			onlyWindows: true,
			opts: &WatchOptions{
				Interval: 0,
				Prompt:   true,
			},
			httpStubs: successfulRunStubs,
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)
			},
			wantOut: "\x1b[2J\x1b[2JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n- trunk more runs · 2\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\x1b[2JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n✓ trunk more runs · 2\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n",
		},
		{
			name:        "exit status respected",
			tty:         true,
			skipWindows: true,
			opts: &WatchOptions{
				Interval:   0,
				Prompt:     true,
				ExitStatus: true,
			},
			httpStubs: failedRunStubs,
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)
			},
			wantOut: "\x1b[2J\x1b[0;0H\x1b[JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n- trunk more runs · 2\nTriggered via push about 59 minutes ago\n\n\x1b[0;0H\x1b[JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\nX trunk more runs · 2\nTriggered via push about 59 minutes ago\n\nJOBS\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nX Run more runs (2) completed with 'failure'\n",
			wantErr: true,
			errMsg:  "SilentError",
		},
		{
			name:        "exit status respected, windows",
			onlyWindows: true,
			opts: &WatchOptions{
				Interval:   0,
				Prompt:     true,
				ExitStatus: true,
			},
			httpStubs: failedRunStubs,
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)
			},
			wantOut: "\x1b[2J\x1b[2JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\n- trunk more runs · 2\nTriggered via push about 59 minutes ago\n\n\x1b[2JRefreshing run status every 0 seconds. Press Ctrl+C to quit.\n\nX trunk more runs · 2\nTriggered via push about 59 minutes ago\n\nJOBS\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n",
			wantErr: true,
			errMsg:  "SilentError",
		},
	}

	for _, tt := range tests {
		if runtime.GOOS == "windows" {
			if tt.skipWindows {
				continue
			}
		} else if tt.onlyWindows {
			continue
		}

		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.Now = func() time.Time {
			notnow, _ := time.Parse("2006-01-02 15:04:05", "2021-02-23 05:50:00")
			return notnow
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdoutTTY(tt.tty)
		tt.opts.IO = io
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("OWNER/REPO")
		}

		as, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(as)
		}

		t.Run(tt.name, func(t *testing.T) {
			err := watchRun(tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
