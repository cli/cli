package run

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/workflow/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRun(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		tty      bool
		wants    RunOptions
		wantsErr bool
		errMsg   string
		stdin    string
	}{
		{
			name:     "blank nontty",
			wantsErr: true,
			errMsg:   "workflow ID, name, or filename required when not running interactively",
		},
		{
			name: "blank tty",
			tty:  true,
			wants: RunOptions{
				Prompt: true,
			},
		},
		{
			name: "ref flag",
			tty:  true,
			cli:  "--ref 12345abc",
			wants: RunOptions{
				Prompt: true,
				Ref:    "12345abc",
			},
		},
		{
			name: "extra args",
			tty:  true,
			cli:  `workflow.yml -- --cool=nah --foo bar`,
			wants: RunOptions{
				InputArgs: []string{"--cool=nah", "--foo", "bar"},
				Selector:  "workflow.yml",
			},
		},
		{
			name:     "both json on STDIN and json arg",
			cli:      `workflow.yml --json '{"cool":"yeah"}'`,
			stdin:    `{"cool":"yeah"}`,
			wantsErr: true,
			errMsg:   "JSON can only be passed on one of STDIN or --json at a time",
		},
		{
			name:     "both json on STDIN and extra args",
			cli:      `workflow.yml -- --cool=nah`,
			stdin:    `{"cool":"yeah"}`,
			errMsg:   "only one of JSON or input arguments can be passed at a time",
			wantsErr: true,
		},
		{
			name:     "both json arg and extra args",
			tty:      true,
			cli:      `workflow.yml --json '{"cool":"yeah"}' -- --cool=nah`,
			errMsg:   "only one of JSON or input arguments can be passed at a time",
			wantsErr: true,
		},
		{
			name: "json via argument",
			cli:  `workflow.yml --json '{"cool":"yeah"}'`,
			tty:  true,
			wants: RunOptions{
				JSON:     `{"cool":"yeah"}`,
				Selector: "workflow.yml",
			},
		},
		{
			name:  "json on STDIN",
			cli:   "workflow.yml",
			stdin: `{"cool":"yeah"}`,
			wants: RunOptions{
				JSON:     `{"cool":"yeah"}`,
				Selector: "workflow.yml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, _, _ := iostreams.Test()
			if tt.stdin == "" {
				io.SetStdinTTY(tt.tty)
			} else {
				stdin.WriteString(tt.stdin)
			}
			io.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *RunOptions
			cmd := NewCmdRun(f, func(opts *RunOptions) error {
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
				if tt.errMsg != "" {
					assert.Equal(t, tt.errMsg, err.Error())
				}
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Selector, gotOpts.Selector)
			assert.Equal(t, tt.wants.Prompt, gotOpts.Prompt)
			assert.Equal(t, tt.wants.JSON, gotOpts.JSON)
			assert.Equal(t, tt.wants.Ref, gotOpts.Ref)
			assert.ElementsMatch(t, tt.wants.InputArgs, gotOpts.InputArgs)
		})
	}
}

func TestRun(t *testing.T) {
	yamlContent := []byte(`
name: a workflow
on:
  workflow_dispatch:
    inputs:
      greeting:
        default: hi
        description: a greeting
      name:
        required: true
        description: a name
jobs:
  greet:
    runs-on: ubuntu-latest
    steps:
      - name: perform the greet
        run: |
          echo "${{ github.event.inputs.greeting}}, ${{ github.events.inputs.name }}!"`)

	encodedYamlContent := base64.StdEncoding.EncodeToString(yamlContent)

	stubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
			httpmock.JSONResponse(shared.Workflow{
				Path: ".github/workflows/workflow.yml",
				ID:   12345,
			}))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
			httpmock.JSONResponse(struct{ Content string }{
				Content: encodedYamlContent,
			}))
		reg.Register(
			httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
			httpmock.StatusStringResponse(204, "cool"))
	}

	tests := []struct {
		name      string
		opts      *RunOptions
		tty       bool
		wantErr   bool
		errOut    string
		wantOut   string
		wantBody  map[string]interface{}
		httpStubs func(*httpmock.Registry)
		askStubs  func(*prompt.AskStubber)
	}{
		{
			name: "bad JSON",
			opts: &RunOptions{
				Selector: "workflow.yml",
				JSON:     `{"bad":"corrupt"`,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
					httpmock.JSONResponse(shared.Workflow{
						Path: ".github/workflows/workflow.yml",
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedYamlContent,
					}))
			},
			wantErr: true,
			errOut:  "could not parse provided JSON: unexpected end of JSON input",
		},
		{
			name: "good JSON",
			tty:  true,
			opts: &RunOptions{
				Selector: "workflow.yml",
				JSON:     `{"name":"scully"}`,
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{
					"name": "scully",
				},
				"ref": "trunk",
			},
			httpStubs: stubs,
			wantOut:   "✓ Created workflow_dispatch event for workflow.yml at trunk\n\nTo see runs for this workflow, try: gh run list --workflow=workflow.yml\n",
		},
		{
			name: "nontty good JSON",
			opts: &RunOptions{
				Selector: "workflow.yml",
				JSON:     `{"name":"scully"}`,
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{
					"name": "scully",
				},
				"ref": "trunk",
			},
			httpStubs: stubs,
		},
		{
			name: "respects ref",
			tty:  true,
			opts: &RunOptions{
				Selector: "workflow.yml",
				JSON:     `{"name":"scully"}`,
				Ref:      "good-branch",
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{
					"name": "scully",
				},
				"ref": "good-branch",
			},
			httpStubs: stubs,
			wantOut:   "✓ Created workflow_dispatch event for workflow.yml at good-branch\n\nTo see runs for this workflow, try: gh run list --workflow=workflow.yml\n",
		},
		{
			name: "good JSON, missing required input",
			tty:  true,
			opts: &RunOptions{
				Selector: "workflow.yml",
				JSON:     `{"greeting":"hello there"}`,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
					httpmock.JSONResponse(shared.Workflow{
						Path: ".github/workflows/workflow.yml",
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedYamlContent,
					}))
			},
			wantErr: true,
			errOut:  "missing required input 'name'",
		},
		{
			name:      "input arguments",
			httpStubs: stubs,
			tty:       true,
			opts: &RunOptions{
				Selector:  "workflow.yml",
				InputArgs: []string{"--name", "scully"},
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{
					"name":     "scully",
					"greeting": "hi",
				},
				"ref": "trunk",
			},
			wantOut: "✓ Created workflow_dispatch event for workflow.yml at trunk\n\nTo see runs for this workflow, try: gh run list --workflow=workflow.yml\n",
		},
		{
			name: "good JSON, missing required input",
			tty:  true,
			opts: &RunOptions{
				Selector:  "workflow.yml",
				InputArgs: []string{"--greeting=hey"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
					httpmock.JSONResponse(shared.Workflow{
						Path: ".github/workflows/workflow.yml",
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedYamlContent,
					}))
			},
			wantErr: true,
			errOut:  "missing required input 'name'",
		},
		{
			name: "good JSON, missing required input",
			tty:  true,
			opts: &RunOptions{
				Selector:  "workflow.yml",
				InputArgs: []string{"--name=scully", "--bad=corrupt"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
					httpmock.JSONResponse(shared.Workflow{
						Path: ".github/workflows/workflow.yml",
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedYamlContent,
					}))
			},
			wantErr: true,
			errOut:  "could not parse input args: unknown flag: --bad",
		},
		{
			name: "prompt, no workflows enabled",
			tty:  true,
			opts: &RunOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							{
								Name:  "disabled",
								State: shared.DisabledManually,
								ID:    102,
							},
						},
					}))
			},
			wantErr: true,
			errOut:  "no workflows are enabled on this repository",
		},
		{
			name: "prompt, no workflows",
			tty:  true,
			opts: &RunOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{},
					}))
			},
			wantErr: true,
			errOut:  "could not fetch workflows for OWNER/REPO: no workflows are enabled",
		},
		{
			name: "prompt",
			tty:  true,
			opts: &RunOptions{
				Prompt: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(shared.WorkflowsPayload{
						Workflows: []shared.Workflow{
							{
								Name:  "a workflow",
								ID:    12345,
								State: shared.Active,
								Path:  ".github/workflows/workflow.yml",
							},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedYamlContent,
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
					httpmock.StatusStringResponse(204, "cool"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)
				as.Stub([]*prompt.QuestionStub{
					{
						Name:    "greeting",
						Default: true,
					},
					{
						Name:  "name",
						Value: "scully",
					},
				})
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{
					"name":     "scully",
					"greeting": "hi",
				},
				"ref": "trunk",
			},
			wantOut: "✓ Created workflow_dispatch event for workflow.yml at trunk\n\nTo see runs for this workflow, try: gh run list --workflow=workflow.yml\n",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdinTTY(tt.tty)
		io.SetStdoutTTY(tt.tty)
		tt.opts.IO = io
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return api.InitRepoHostname(&api.Repository{
				Name:             "REPO",
				Owner:            api.RepositoryOwner{Login: "OWNER"},
				DefaultBranchRef: api.BranchRef{Name: "trunk"},
			}, "github.com"), nil
		}

		as, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(as)
		}
		t.Run(tt.name, func(t *testing.T) {
			err := runRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errOut, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)

			if len(reg.Requests) > 0 {
				lastRequest := reg.Requests[len(reg.Requests)-1]
				if lastRequest.Method == "POST" {
					bodyBytes, _ := ioutil.ReadAll(lastRequest.Body)
					reqBody := make(map[string]interface{})
					err := json.Unmarshal(bodyBytes, &reqBody)
					if err != nil {
						t.Fatalf("error decoding JSON: %v", err)
					}
					assert.Equal(t, tt.wantBody, reqBody)
				}
			}
		})
	}
}
