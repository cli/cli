package view

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmd/workflow/shared"
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
			name: "blank tty",
			tty:  true,
			wants: ViewOptions{
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
			wants: ViewOptions{
				WorkflowSelector: "123",
			},
		},
		{
			name: "arg nontty",
			cli:  "123",
			wants: ViewOptions{
				WorkflowSelector: "123",
				Raw:              true,
			},
		},
		{
			name: "web tty",
			cli:  "--web",
			tty:  true,
			wants: ViewOptions{
				Prompt: true,
				Web:    true,
			},
		},
		{
			name: "web nontty",
			cli:  "-w 123",
			wants: ViewOptions{
				Raw:              true,
				Web:              true,
				WorkflowSelector: "123",
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

			assert.Equal(t, tt.wants.WorkflowSelector, gotOpts.WorkflowSelector)
			assert.Equal(t, tt.wants.Prompt, gotOpts.Prompt)
			assert.Equal(t, tt.wants.Raw, gotOpts.Raw)
			assert.Equal(t, tt.wants.Web, gotOpts.Web)
		})
	}
}

func TestViewRun(t *testing.T) {
	aWorkflow := shared.Workflow{
		Name:  "a workflow",
		ID:    123,
		Path:  ".github/workflows/flow.yml",
		State: shared.Active,
	}
	aWorkflowContent := `{"content":"bmFtZTogYSB3b3JrZmxvdwo="}`
	disabledWorkflow := shared.Workflow{
		Name:  "a disabled workflow",
		ID:    456,
		Path:  ".github/workflows/disabled.yml",
		State: shared.DisabledManually,
	}
	anotherWorkflow := shared.Workflow{
		Name:  "another workflow",
		ID:    789,
		Path:  ".github/workflows/another.yml",
		State: shared.Active,
	}
	anotherWorkflowContent := `{"content":"bmFtZTogYW5vdGhlciB3b3JrZmxvdwo="}`
	yetAnotherWorkflow := shared.Workflow{
		Name:  "another workflow",
		ID:    1011,
		Path:  ".github/workflows/yetanother.yml",
		State: shared.Active,
	}

	tests := []struct {
		name       string
		opts       *ViewOptions
		httpStubs  func(*httpmock.Registry)
		askStubs   func(*prompt.AskStubber)
		tty        bool
		wantOut    string
		wantErrOut string
		wantErr    bool
	}{
		{
			name: "tty, no workflows",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{}))
			},
			wantErrOut: "could not fetch workflows for OWNER/REPO: no workflows are enabled",
			wantErr:    true,
		},
		{
			name: "tty, workflows",
			tty:  true,
			opts: &ViewOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							aWorkflow,
							disabledWorkflow,
							anotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/another.yml"),
					httpmock.StringResponse(anotherWorkflowContent))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)
			},
			wantOut: "another workflow\n.github/workflows/another.yml ID: 789\n\nname: another workflow\n\n\n",
		},
		{
			name: "tty, name, nonunique workflow",
			tty:  true,
			opts: &ViewOptions{
				WorkflowSelector: "another workflow",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/another workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							aWorkflow,
							disabledWorkflow,
							anotherWorkflow,
							yetAnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/another.yml"),
					httpmock.StringResponse(anotherWorkflowContent))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)
			},
			wantOut: "another workflow\n.github/workflows/another.yml ID: 789\n\nname: another workflow\n\n\n",
		},
		{
			name: "tty, name, unique workflow",
			tty:  true,
			opts: &ViewOptions{
				WorkflowSelector: "a workflow",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/a workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							aWorkflow,
							disabledWorkflow,
							anotherWorkflow,
							yetAnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StringResponse(aWorkflowContent))
			},
			wantOut: "a workflow\n.github/workflows/flow.yml ID: 123\n\nname: a workflow\n\n\n",
		},
		{
			name: "nontty, name, unique workflow",
			opts: &ViewOptions{
				WorkflowSelector: "a workflow",
				Raw:              true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/a workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							aWorkflow,
							disabledWorkflow,
							anotherWorkflow,
							yetAnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StringResponse(aWorkflowContent))
			},
			wantOut: "name: a workflow\n",
		},
		{
			name: "nontty, name, nonunique workflow",
			opts: &ViewOptions{
				WorkflowSelector: "another workflow",
				Raw:              true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/another workflow"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							aWorkflow,
							disabledWorkflow,
							anotherWorkflow,
							yetAnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/another.yml"),
					httpmock.StringResponse(anotherWorkflowContent))
			},
			wantErrOut: "could not resolve to a unique workflow; found: another.yml yetanother.yml",
			wantErr:    true,
		},
		{
			name: "nontty ID",
			opts: &ViewOptions{
				WorkflowSelector: "123",
				Raw:              true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StringResponse(aWorkflowContent))
			},
			wantOut: "name: a workflow\n",
		},
		{
			name: "tty ID",
			tty:  true,
			opts: &ViewOptions{
				WorkflowSelector: "123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StringResponse(aWorkflowContent))
			},
			wantOut: "a workflow\n.github/workflows/flow.yml ID: 123\n\nname: a workflow\n\n\n",
		},
		{
			name: "web ID",
			tty:  true,
			opts: &ViewOptions{
				WorkflowSelector: "123",
				Web:              true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow))
			},
			wantOut: "Opening github.com/OWNER/REPO/actions/workflows/flow.yml in your browser.\n",
		},
		{
			name: "web ID",
			opts: &ViewOptions{
				Raw:              true,
				WorkflowSelector: "123",
				Web:              true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow))
			},
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
			cs, teardown := run.Stub()
			defer teardown(t)
			if tt.opts.Web {
				cs.Register(`https://github\.com/OWNER/REPO/actions/workflows/flow.yml$`, 0, "")
			}

			err := runView(tt.opts)
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
