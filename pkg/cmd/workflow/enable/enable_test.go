package enable

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/workflow/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdEnable(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		tty      bool
		wants    EnableOptions
		wantsErr bool
	}{
		{
			name: "blank tty",
			tty:  true,
			wants: EnableOptions{
				Prompt: true,
			},
		},
		{
			name:     "blank nontty",
			wantsErr: true,
		},
		{
			name: "arg tty",
			cli:  "123",
			tty:  true,
			wants: EnableOptions{
				Selector: "123",
			},
		},
		{
			name: "arg nontty",
			cli:  "123",
			wants: EnableOptions{
				Selector: "123",
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

			var gotOpts *EnableOptions
			cmd := NewCmdEnable(f, func(opts *EnableOptions) error {
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

			assert.Equal(t, tt.wants.Selector, gotOpts.Selector)
			assert.Equal(t, tt.wants.Prompt, gotOpts.Prompt)
		})
	}
}

func TestEnableRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *EnableOptions
		httpStubs  func(*httpmock.Registry)
		askStubs   func(*prompt.AskStubber)
		tty        bool
		wantOut    string
		wantErrOut string
		wantErr    bool
	}{
		{
			name: "tty no arg",
			opts: &EnableOptions{
				Prompt: true,
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							shared.AWorkflow,
							shared.DisabledWorkflow,
							shared.AnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/456/enable"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)
			},
			wantOut: "✓ Enabled a disabled workflow\n",
		},
		{
			name: "tty name arg",
			opts: &EnableOptions{
				Selector: "terrible workflow",
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/terrible workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							shared.AWorkflow,
							shared.DisabledWorkflow,
							shared.UniqueDisabledWorkflow,
							shared.AnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/1314/enable"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			wantOut: "✓ Enabled terrible workflow\n",
		},
		{
			name: "tty name arg nonunique",
			opts: &EnableOptions{
				Selector: "a disabled workflow",
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/a disabled workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							shared.AWorkflow,
							shared.DisabledWorkflow,
							shared.AnotherWorkflow,
							shared.AnotherDisabledWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/1213/enable"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)
			},
			wantOut: "✓ Enabled a disabled workflow\n",
		},
		{
			name: "tty ID arg",
			opts: &EnableOptions{
				Selector: "456",
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/456"),
					httpmock.JSONResponse(shared.DisabledWorkflow))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/456/enable"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)
			},
			wantOut: "✓ Enabled a disabled workflow\n",
		},
		{
			name: "nontty ID arg",
			opts: &EnableOptions{
				Selector: "456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/456"),
					httpmock.JSONResponse(shared.DisabledWorkflow))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/456/enable"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "nontty name arg",
			opts: &EnableOptions{
				Selector: "terrible workflow",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/terrible workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							shared.AWorkflow,
							shared.DisabledWorkflow,
							shared.AnotherWorkflow,
							shared.AnotherDisabledWorkflow,
							shared.UniqueDisabledWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/1314/enable"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "nontty name arg nonunique",
			opts: &EnableOptions{
				Selector: "a disabled workflow",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/a disabled workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							shared.AWorkflow,
							shared.DisabledWorkflow,
							shared.AnotherWorkflow,
							shared.AnotherDisabledWorkflow,
							shared.UniqueDisabledWorkflow,
						},
					}))
			},
			wantErr:    true,
			wantErrOut: "could not resolve to a unique workflow; found: disabled.yml anotherDisabled.yml",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdoutTTY(tt.tty)
		io.SetStdinTTY(tt.tty)
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
			err := runEnable(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErrOut, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
