package cancel

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdCancel(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		tty      bool
		wants    CancelOptions
		wantsErr bool
	}{
		{
			name: "blank tty",
			tty:  true,
			wants: CancelOptions{
				Prompt: true,
			},
		},
		{
			name:     "blank nontty",
			wantsErr: true,
		},
		{
			name: "with arg",
			cli:  "1234",
			wants: CancelOptions{
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

			var gotOpts *CancelOptions
			cmd := NewCmdCancel(f, func(opts *CancelOptions) error {
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
		})
	}
}

func TestRunCancel(t *testing.T) {
	inProgressRun := shared.TestRun("more runs", 1234, shared.InProgress, "")
	completedRun := shared.TestRun("more runs", 4567, shared.Completed, shared.Failure)
	tests := []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		askStubs  func(*prompt.AskStubber)
		opts      *CancelOptions
		wantErr   bool
		wantOut   string
		errMsg    string
	}{
		{
			name: "cancel run",
			opts: &CancelOptions{
				RunID: "1234",
			},
			wantErr: false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(inProgressRun))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/cancel"),
					httpmock.StatusStringResponse(202, "{}"))
			},
			wantOut: "✓ Request to cancel workflow submitted.\n",
		},
		{
			name: "not found",
			opts: &CancelOptions{
				RunID: "1234",
			},
			wantErr: true,
			errMsg:  "Could not find any workflow run with ID 1234",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.StatusStringResponse(404, ""))
			},
		},
		{
			name: "completed",
			opts: &CancelOptions{
				RunID: "4567",
			},
			wantErr: true,
			errMsg:  "Cannot cancel a workflow run that is completed",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/4567"),
					httpmock.JSONResponse(completedRun))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/4567/cancel"),
					httpmock.StatusStringResponse(409, ""),
				)
			},
		},
		{
			name: "prompt, no in progress runs",
			opts: &CancelOptions{
				Prompt: true,
			},
			wantErr: true,
			errMsg:  "found no in progress runs to cancel",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: []shared.Run{
							completedRun,
						},
					}))
			},
		},
		{
			name: "prompt, cancel",
			opts: &CancelOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: []shared.Run{
							inProgressRun,
						},
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/cancel"),
					httpmock.StatusStringResponse(202, "{}"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)
			},
			wantOut: "✓ Request to cancel workflow submitted.\n",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdoutTTY(true)
		io.SetStdinTTY(true)
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
			err := runCancel(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Equal(t, tt.errMsg, err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
