package view

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	runShared "github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
				Selector: "123",
			},
		},
		{
			name: "arg nontty",
			cli:  "123",
			wants: ViewOptions{
				Selector: "123",
				Raw:      true,
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
				Raw:      true,
				Web:      true,
				Selector: "123",
			},
		},
		{
			name: "yaml tty",
			cli:  "--yaml",
			tty:  true,
			wants: ViewOptions{
				Prompt: true,
				YAML:   true,
			},
		},
		{
			name: "yaml nontty",
			cli:  "-y 123",
			wants: ViewOptions{
				Raw:      true,
				YAML:     true,
				Selector: "123",
			},
		},
		{
			name:     "ref tty",
			cli:      "--ref 456",
			tty:      true,
			wantsErr: true,
		},
		{
			name:     "ref nontty",
			cli:      "123 -r 456",
			wantsErr: true,
		},
		{
			name: "yaml ref tty",
			cli:  "--yaml --ref 456",
			tty:  true,
			wants: ViewOptions{
				Prompt: true,
				YAML:   true,
				Ref:    "456",
			},
		},
		{
			name: "yaml ref nontty",
			cli:  "123 -y -r 456",
			wants: ViewOptions{
				Raw:      true,
				YAML:     true,
				Ref:      "456",
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
			assert.Equal(t, tt.wants.Selector, gotOpts.Selector)
			assert.Equal(t, tt.wants.Ref, gotOpts.Ref)
			assert.Equal(t, tt.wants.Web, gotOpts.Web)
			assert.Equal(t, tt.wants.Prompt, gotOpts.Prompt)
			assert.Equal(t, tt.wants.Raw, gotOpts.Raw)
			assert.Equal(t, tt.wants.YAML, gotOpts.YAML)
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
	aWorkflowInfo := heredoc.Doc(`
		a workflow - flow.yml
		ID: 123

		Total runs 10
		Recent runs
		X  cool commit  a workflow  trunk  push  1
		*  cool commit  a workflow  trunk  push  2
		✓  cool commit  a workflow  trunk  push  3
		X  cool commit  a workflow  trunk  push  4

		To see more runs for this workflow, try: gh run list --workflow flow.yml
		To see the YAML for this workflow, try: gh workflow view flow.yml --yaml
	`)

	tests := []struct {
		name       string
		opts       *ViewOptions
		httpStubs  func(*httpmock.Registry)
		tty        bool
		wantOut    string
		wantErrOut string
		wantErr    bool
	}{
		{
			name: "no enabled workflows",
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
			name: "web",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
				Web:      true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
			},
			wantOut: "Opening github.com/OWNER/REPO/actions/workflows/flow.yml in your browser.\n",
		},
		{
			name: "web notty",
			tty:  false,
			opts: &ViewOptions{
				Selector: "123",
				Web:      true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
			},
			wantOut: "",
		},
		{
			name: "web with yaml",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
				YAML:     true,
				Web:      true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{ "data": { "repository": { "defaultBranchRef": { "name": "trunk" } } } }`),
				)
			},
			wantOut: "Opening github.com/OWNER/REPO/blob/trunk/.github/workflows/flow.yml in your browser.\n",
		},
		{
			name: "web with yaml and ref",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
				Ref:      "base",
				YAML:     true,
				Web:      true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
			},
			wantOut: "Opening github.com/OWNER/REPO/blob/base/.github/workflows/flow.yml in your browser.\n",
		},
		{
			name: "workflow with yaml",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
				YAML:     true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StringResponse(aWorkflowContent),
				)
			},
			wantOut: "a workflow - flow.yml\nID: 123\n\nname: a workflow\n\n\n",
		},
		{
			name: "workflow with yaml notty",
			tty:  false,
			opts: &ViewOptions{
				Selector: "123",
				YAML:     true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StringResponse(aWorkflowContent),
				)
			},
			wantOut: "a workflow - flow.yml\nID: 123\n\nname: a workflow\n\n\n",
		},
		{
			name: "workflow with yaml not found",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
				YAML:     true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StatusStringResponse(404, "not Found"),
				)
			},
			wantErr:    true,
			wantErrOut: "could not find workflow file flow.yml, try specifying a branch or tag using `--ref`",
		},
		{
			name: "workflow with yaml and ref",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
				Ref:      "456",
				YAML:     true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/flow.yml"),
					httpmock.StringResponse(aWorkflowContent),
				)
			},
			wantOut: "a workflow - flow.yml\nID: 123\n\nname: a workflow\n\n\n",
		},
		{
			name: "workflow info",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123/runs"),
					httpmock.JSONResponse(runShared.RunsPayload{
						TotalCount:   10,
						WorkflowRuns: runShared.TestRuns[0:4],
					}),
				)
			},
			wantOut: aWorkflowInfo,
		},
		{
			name: "workflow info notty",
			tty:  true,
			opts: &ViewOptions{
				Selector: "123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(aWorkflow),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123/runs"),
					httpmock.JSONResponse(runShared.RunsPayload{
						TotalCount:   10,
						WorkflowRuns: runShared.TestRuns[0:4],
					}),
				)
			},
			wantOut: aWorkflowInfo,
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

		browser := &browser.Stub{}
		tt.opts.Browser = browser

		t.Run(tt.name, func(t *testing.T) {
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
