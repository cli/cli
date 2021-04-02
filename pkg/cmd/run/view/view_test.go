package view

import (
	"bytes"
	"io/ioutil"
	"net/http"
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

			var gotOpts *ViewOptions
			cmd := NewCmdView(f, func(opts *ViewOptions) error {
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
			assert.Equal(t, tt.wants.Verbose, gotOpts.Verbose)
		})
	}
}

func TestViewRun(t *testing.T) {

	tests := []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		askStubs  func(*prompt.AskStubber)
		opts      *ViewOptions
		tty       bool
		wantErr   bool
		wantOut   string
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
			wantOut: "\n✓ trunk successful #2898 · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about a job, try: gh run view --job=<job-id>\nview this run on GitHub: runs/3\n",
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
			wantOut: "\nX trunk failed · 1234\nTriggered via push about 59 minutes ago\n\nJOBS\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nFor more information about a job, try: gh run view --job=<job-id>\nview this run on GitHub: runs/1234\n",
			wantErr: true,
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
			},
			wantOut: "\n✓ trunk successful · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about a job, try: gh run view --job=<job-id>\nview this run on GitHub: runs/3\n",
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
			},
			wantOut: "\nX trunk failed · 1234\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nFor more information about a job, try: gh run view --job=<job-id>\nview this run on GitHub: runs/1234\n",
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
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
			},
			opts: &ViewOptions{
				Prompt: true,
			},
			wantOut: "\n✓ trunk successful · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\n\nFor more information about a job, try: gh run view --job=<job-id>\nview this run on GitHub: runs/3\n",
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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("it's a log\nfor this job\nbeautiful log\n"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
				as.StubOne(1)
			},
			wantOut: "it's a log\nfor this job\nbeautiful log\n",
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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("it's a log\nfor this job\nbeautiful log\n"))
			},
			wantOut: "it's a log\nfor this job\nbeautiful log\n",
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
			},
			wantOut: "\n✓ trunk successful · 3\nTriggered via push about 59 minutes ago\n\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\nTo see the full job log, try: gh run view --log --job=10\nview this run on GitHub: runs/3\n",
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
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
				as.StubOne(0)
			},
			wantOut: "\n✓ trunk successful · 3\nTriggered via push about 59 minutes ago\n\nJOBS\n✓ cool job in 4m34s (ID 10)\nX sad job in 4m34s (ID 20)\n  ✓ barf the quux\n  X quux the barf\n\nANNOTATIONS\nX the job is sad\nsad job: blaze.py#420\n\n\nFor more information about a job, try: gh run view --job=<job-id>\nview this run on GitHub: runs/3\n",
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
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
				as.StubOne(1)
			},
			wantOut: "\n✓ trunk successful · 3\nTriggered via push about 59 minutes ago\n\n✓ cool job in 4m34s (ID 10)\n  ✓ fob the barz\n  ✓ barz the fob\n\nTo see the full job log, try: gh run view --log --job=10\nview this run on GitHub: runs/3\n",
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
			err := runView(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				if !tt.opts.ExitStatus {
					return
				}
			}
			if !tt.opts.ExitStatus {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
