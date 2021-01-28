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
		wants    ViewOptions
		wantsErr bool
		tty      bool
	}{
		{
			name: "blank tty",
			tty:  true,
			wants: ViewOptions{
				Prompt:       true,
				ShowProgress: true,
			},
		},
		{
			name:     "blank nontty",
			wantsErr: true,
		},
		{
			name: "nontty jobID",
			cli:  "1234",
			wants: ViewOptions{
				JobID: "1234",
			},
		},
		{
			name: "log tty",
			tty:  true,
			cli:  "--log",
			wants: ViewOptions{
				Prompt:       true,
				Log:          true,
				ShowProgress: true,
			},
		},
		{
			name: "log nontty",
			cli:  "--log 1234",
			wants: ViewOptions{
				JobID: "1234",
				Log:   true,
			},
		},
		{
			name: "exit status",
			cli:  "-e 1234",
			wants: ViewOptions{
				JobID:      "1234",
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

			assert.Equal(t, tt.wants.JobID, gotOpts.JobID)
			assert.Equal(t, tt.wants.Log, gotOpts.Log)
			assert.Equal(t, tt.wants.Prompt, gotOpts.Prompt)
			assert.Equal(t, tt.wants.ExitStatus, gotOpts.ExitStatus)
			assert.Equal(t, tt.wants.ShowProgress, gotOpts.ShowProgress)
		})
	}
}

func TestRunView(t *testing.T) {
	tests := []struct {
		name      string
		opts      *ViewOptions
		httpStubs func(*httpmock.Registry)
		askStubs  func(*prompt.AskStubber)
		tty       bool
		wantErr   bool
		wantOut   string
	}{
		{
			name: "interactive flow, multi-job",
			tty:  true,
			opts: &ViewOptions{
				Prompt:       true,
				ShowProgress: true,
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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
				as.StubOne(0)
			},
			wantOut: "\n\ncool job (ID 10)\n✓ about 59 minutes ago in 4m34s\n\n✓ fob the barz\n✓ barz the fob\n\nTo see the full logs for this job, try: gh job view 10 --log\nView this job on GitHub: jobs/10\n",
		},
		{
			name: "interactive, run has only one job",
			tty:  true,
			opts: &ViewOptions{
				Prompt:       true,
				ShowProgress: true,
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
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
			},
			wantOut: "\n\ncool job (ID 10)\n✓ about 59 minutes ago in 4m34s\n\n✓ fob the barz\n✓ barz the fob\n\nTo see the full logs for this job, try: gh job view 10 --log\nView this job on GitHub: jobs/10\n",
		},
		{
			name: "interactive with log",
			tty:  true,
			opts: &ViewOptions{
				Prompt:       true,
				ShowProgress: true,
				Log:          true,
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
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("it's a log\nfor this job\nbeautiful log\n"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
			},
			wantOut: "\n\nit's a log\nfor this job\nbeautiful log\n",
		},
		{
			name: "noninteractive with log",
			opts: &ViewOptions{
				JobID:        "10",
				Prompt:       false,
				ShowProgress: false,
				Log:          true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10/logs"),
					httpmock.StringResponse("it's a log\nfor this job\nbeautiful log\n"))
			},
			wantOut: "it's a log\nfor this job\nbeautiful log\n",
		},
		{
			name: "noninteractive",
			opts: &ViewOptions{
				JobID: "10",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/10"),
					httpmock.JSONResponse(shared.SuccessfulJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/10/annotations"),
					httpmock.JSONResponse([]shared.Annotation{}))
			},
			wantOut: "cool job (ID 10)\n✓ about 59 minutes ago in 4m34s\n\n✓ fob the barz\n✓ barz the fob\n\nTo see the full logs for this job, try: gh job view 10 --log\nView this job on GitHub: jobs/10\n",
		},
		{
			name: "shows annotations for failed job",
			opts: &ViewOptions{
				JobID: "20",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20"),
					httpmock.JSONResponse(shared.FailedJob))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/check-runs/20/annotations"),
					httpmock.JSONResponse(shared.FailedJobAnnotations))
			},
			wantOut: "sad job (ID 20)\nX about 59 minutes ago in 4m34s\n\n✓ barf the quux\nX quux the barf\n\nANNOTATIONS\nX the job is sad\nblaze.py#420\n\n\nTo see the full logs for this job, try: gh job view 20 --log\nView this job on GitHub: jobs/20\n",
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
