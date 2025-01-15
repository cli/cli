package enable

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

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
		name        string
		opts        *EnableOptions
		httpStubs   func(*httpmock.Registry)
		promptStubs func(*prompter.MockPrompter)
		tty         bool
		wantOut     string
		wantErrOut  string
		wantErr     bool
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
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow", []string{"a disabled workflow (disabled.yml)"}, func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "a disabled workflow (disabled.yml)")
				})
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
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Which workflow do you mean?", []string{"a disabled workflow (disabled.yml)", "a disabled workflow (anotherDisabled.yml)"}, func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "a disabled workflow (anotherDisabled.yml)")
				})
			},
			wantOut: "✓ Enabled a disabled workflow\n",
		},
		{
			name: "tty name arg inactivity workflow",
			opts: &EnableOptions{
				Selector: "a disabled inactivity workflow",
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							shared.AWorkflow,
							shared.DisabledInactivityWorkflow,
							shared.UniqueDisabledWorkflow,
							shared.AnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/1206/enable"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			wantOut: "✓ Enabled a disabled inactivity workflow\n",
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
			name: "nontty name arg inactivity workflow",
			opts: &EnableOptions{
				Selector: "a disabled inactivity workflow",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							shared.AWorkflow,
							shared.DisabledInactivityWorkflow,
							shared.UniqueDisabledWorkflow,
							shared.AnotherWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/actions/workflows/1206/enable"),
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

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(tt.tty)
		ios.SetStdinTTY(tt.tty)
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
