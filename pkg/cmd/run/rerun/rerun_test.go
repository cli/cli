package rerun

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRerun(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		tty      bool
		wants    RerunOptions
		wantsErr bool
	}{
		{
			name:     "blank nontty",
			wantsErr: true,
		},
		{
			name: "blank tty",
			tty:  true,
			wants: RerunOptions{
				Prompt: true,
			},
		},
		{
			name: "with arg nontty",
			cli:  "1234",
			wants: RerunOptions{
				RunID: "1234",
			},
		},
		{
			name: "with arg tty",
			tty:  true,
			cli:  "1234",
			wants: RerunOptions{
				RunID: "1234",
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

			var gotOpts *RerunOptions
			cmd := NewCmdRerun(f, func(opts *RerunOptions) error {
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
		})
	}

}

func TestRerun(t *testing.T) {
	tests := []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		askStubs  func(*prompt.AskStubber)
		opts      *RerunOptions
		tty       bool
		wantErr   bool
		ErrOut    string
		wantOut   string
	}{
		{
			name: "arg",
			tty:  true,
			opts: &RerunOptions{
				RunID: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/rerun"),
					httpmock.StringResponse("{}"))
			},
			wantOut: "✓ Requested rerun of run 1234\n",
		},
		{
			name: "prompt",
			tty:  true,
			opts: &RerunOptions{
				Prompt: true,
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
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/rerun"),
					httpmock.StringResponse("{}"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(2)
			},
			wantOut: "✓ Requested rerun of run 1234\n",
		},
		{
			name: "prompt but no failed runs",
			tty:  true,
			opts: &RerunOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: []shared.Run{
							shared.SuccessfulRun,
							shared.TestRun("in progress", 2, shared.InProgress, ""),
						}}))
			},
			wantErr: true,
			ErrOut:  "no recent runs have failed; please specify a specific run ID",
		},
		{
			name: "unrerunnable",
			tty:  true,
			opts: &RerunOptions{
				RunID: "3",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/3"),
					httpmock.JSONResponse(shared.SuccessfulRun))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/3/rerun"),
					httpmock.StatusStringResponse(403, "no"))
			},
			wantErr: true,
			ErrOut:  "run 3 cannot be rerun; its workflow file may be broken.",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdinTTY(tt.tty)
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
			err := runRerun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.ErrOut, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
