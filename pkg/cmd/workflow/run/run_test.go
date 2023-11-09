package run

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			name:     "both STDIN and input fields",
			stdin:    "some json",
			cli:      "workflow.yml -fhey=there --json",
			errMsg:   "only one of STDIN or -f/-F can be passed",
			wantsErr: true,
		},
		{
			name: "-f args",
			tty:  true,
			cli:  `workflow.yml -fhey=there -fname="dana scully"`,
			wants: RunOptions{
				Selector:  "workflow.yml",
				RawFields: []string{"hey=there", "name=dana scully"},
			},
		},
		{
			name: "-F args",
			tty:  true,
			cli:  `workflow.yml -Fhey=there -Fname="dana scully" -Ffile=@cool.txt`,
			wants: RunOptions{
				Selector:    "workflow.yml",
				MagicFields: []string{"hey=there", "name=dana scully", "file=@cool.txt"},
			},
		},
		{
			name: "-F/-f arg mix",
			tty:  true,
			cli:  `workflow.yml -fhey=there -Fname="dana scully" -Ffile=@cool.txt`,
			wants: RunOptions{
				Selector:    "workflow.yml",
				RawFields:   []string{"hey=there"},
				MagicFields: []string{`name=dana scully`, "file=@cool.txt"},
			},
		},
		{
			name:  "json on STDIN",
			cli:   "workflow.yml --json",
			stdin: `{"cool":"yeah"}`,
			wants: RunOptions{
				JSON:      true,
				JSONInput: `{"cool":"yeah"}`,
				Selector:  "workflow.yml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			if tt.stdin == "" {
				ios.SetStdinTTY(tt.tty)
			} else {
				stdin.WriteString(tt.stdin)
			}
			ios.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

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
			assert.Equal(t, tt.wants.JSONInput, gotOpts.JSONInput)
			assert.Equal(t, tt.wants.JSON, gotOpts.JSON)
			assert.Equal(t, tt.wants.Ref, gotOpts.Ref)
			assert.ElementsMatch(t, tt.wants.RawFields, gotOpts.RawFields)
			assert.ElementsMatch(t, tt.wants.MagicFields, gotOpts.MagicFields)
		})
	}
}

func Test_magicFieldValue(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "gh-test")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	fmt.Fprint(f, "file contents")

	ios, _, _, _ := iostreams.Test()

	type args struct {
		v    string
		opts RunOptions
	}
	tests := []struct {
		name    string
		args    args
		want    interface{}
		wantErr bool
	}{
		{
			name:    "string",
			args:    args{v: "hello"},
			want:    "hello",
			wantErr: false,
		},
		{
			name: "file",
			args: args{
				v:    "@" + f.Name(),
				opts: RunOptions{IO: ios},
			},
			want:    "file contents",
			wantErr: false,
		},
		{
			name: "file error",
			args: args{
				v:    "@",
				opts: RunOptions{IO: ios},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := magicFieldValue(tt.args.v, tt.args.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("magicFieldValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_findInputs(t *testing.T) {
	tests := []struct {
		name    string
		YAML    []byte
		wantErr bool
		errMsg  string
		wantOut []WorkflowInput
	}{
		{
			name:    "blank",
			YAML:    []byte{},
			wantErr: true,
			errMsg:  "invalid YAML file",
		},
		{
			name:    "no event specified",
			YAML:    []byte("name: workflow"),
			wantErr: true,
			errMsg:  "invalid workflow: no 'on' key",
		},
		{
			name:    "not workflow_dispatch",
			YAML:    []byte("name: workflow\non: pull_request"),
			wantErr: true,
			errMsg:  "unable to manually run a workflow without a workflow_dispatch event",
		},
		{
			name:    "bad inputs",
			YAML:    []byte("name: workflow\non:\n workflow_dispatch:\n  inputs: lol  "),
			wantErr: true,
			errMsg:  "could not decode workflow inputs: yaml: unmarshal errors:\n  line 4: cannot unmarshal !!str `lol` into map[string]run.WorkflowInput",
		},
		{
			name:    "short syntax",
			YAML:    []byte("name: workflow\non: workflow_dispatch"),
			wantOut: []WorkflowInput{},
		},
		{
			name:    "array of events",
			YAML:    []byte("name: workflow\non: [pull_request, workflow_dispatch]\n"),
			wantOut: []WorkflowInput{},
		},
		{
			name: "inputs",
			YAML: []byte(`name: workflow
on:
  workflow_dispatch:
    inputs:
      foo:
        required: true
        description: good foo
      bar:
        default: boo
      baz:
        description: it's baz
      quux:
        required: true
        default: "cool"
jobs:
  yell:
    runs-on: ubuntu-latest
    steps:
      - name: echo
        run: |
          echo "echo"`),
			wantOut: []WorkflowInput{
				{
					Name:    "bar",
					Default: "boo",
				},
				{
					Name:        "baz",
					Description: "it's baz",
				},
				{
					Name:        "foo",
					Required:    true,
					Description: "good foo",
				},
				{
					Name:     "quux",
					Required: true,
					Default:  "cool",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := findInputs(tt.YAML)
			if tt.wantErr {
				assert.Error(t, err)
				if err != nil {
					assert.Equal(t, tt.errMsg, err.Error())
				}
				return
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantOut, result)
		})
	}

}

func TestRun(t *testing.T) {
	noInputsYAMLContent := []byte(`
name: minimal workflow
on: workflow_dispatch
jobs:
  yell:
    runs-on: ubuntu-latest
    steps:
      - name: do a yell
        run: |
          echo "AUUUGH!"
`)
	encodedNoInputsYAMLContent := base64.StdEncoding.EncodeToString(noInputsYAMLContent)
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

	encodedYAMLContent := base64.StdEncoding.EncodeToString(yamlContent)

	yamlContentChoiceIp := []byte(`
name: choice inputs
on:
  workflow_dispatch:
    inputs:
      name:
        type: choice
        description: Who to greet
        default: monalisa
        options:
        - monalisa
        - cschleiden
      favourite-animal:
        type: choice
        description: What's your favourite animal
        required: true
        options:
        - dog
        - cat
jobs:
  greet:
  runs-on: ubuntu-latest
  steps:
  - name: Send greeting
    run: echo "${{ github.event.inputs.message }} ${{ fromJSON('["", "🥳"]')[github.event.inputs.use-emoji == 'true'] }} ${{ github.event.inputs.name }}"`)
	encodedYAMLContentChoiceIp := base64.StdEncoding.EncodeToString(yamlContentChoiceIp)

	yamlContentMissingChoiceIp := []byte(`
name: choice missing inputs
on:
  workflow_dispatch:
    inputs:
      name:
        type: choice
        description: Who to greet
        options:
jobs:
  greet:
  runs-on: ubuntu-latest
  steps:
  - name: Send greeting
    run: echo "${{ github.event.inputs.message }} ${{ fromJSON('["", "🥳"]')[github.event.inputs.use-emoji == 'true'] }} ${{ github.event.inputs.name }}"`)
	encodedYAMLContentMissingChoiceIp := base64.StdEncoding.EncodeToString(yamlContentMissingChoiceIp)

	stubs := func(reg *httpmock.Registry) {
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
			httpmock.JSONResponse(shared.Workflow{
				Path: ".github/workflows/workflow.yml",
				ID:   12345,
			}))
		reg.Register(
			httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
			httpmock.StatusStringResponse(204, "cool"))
	}

	tests := []struct {
		name        string
		opts        *RunOptions
		tty         bool
		wantErr     bool
		errOut      string
		wantOut     string
		wantBody    map[string]interface{}
		httpStubs   func(*httpmock.Registry)
		promptStubs func(*prompter.MockPrompter)
	}{
		{
			name: "bad JSON",
			opts: &RunOptions{
				Selector:  "workflow.yml",
				JSONInput: `{"bad":"corrupt"`,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
					httpmock.JSONResponse(shared.Workflow{
						Path: ".github/workflows/workflow.yml",
					}))
			},
			wantErr: true,
			errOut:  "could not parse provided JSON: unexpected end of JSON input",
		},
		{
			name: "good JSON",
			tty:  true,
			opts: &RunOptions{
				Selector:  "workflow.yml",
				JSONInput: `{"name":"scully"}`,
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
				Selector:  "workflow.yml",
				JSONInput: `{"name":"scully"}`,
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
			name: "nontty good input fields",
			opts: &RunOptions{
				Selector:    "workflow.yml",
				RawFields:   []string{`name=scully`},
				MagicFields: []string{`greeting=hey`},
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{
					"name":     "scully",
					"greeting": "hey",
				},
				"ref": "trunk",
			},
			httpStubs: stubs,
		},
		{
			name: "respects ref",
			tty:  true,
			opts: &RunOptions{
				Selector:  "workflow.yml",
				JSONInput: `{"name":"scully"}`,
				Ref:       "good-branch",
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
			// TODO this test is somewhat silly; it's more of a placeholder in case I decide to handle the API error more elegantly
			name: "good JSON, missing required input",
			tty:  true,
			opts: &RunOptions{
				Selector:  "workflow.yml",
				JSONInput: `{"greeting":"hello there"}`,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
					httpmock.JSONResponse(shared.Workflow{
						Path: ".github/workflows/workflow.yml",
						ID:   12345,
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
					httpmock.StatusStringResponse(422, "missing something"))
			},
			wantErr: true,
			errOut:  "could not create workflow dispatch event: HTTP 422 (https://api.github.com/repos/OWNER/REPO/actions/workflows/12345/dispatches)",
		},
		{
			name: "yaml file extension",
			tty:  false,
			opts: &RunOptions{
				Selector: "workflow.yaml",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yaml"),
					httpmock.StatusStringResponse(200, `{"id": 12345}`))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
					httpmock.StatusStringResponse(204, ""))
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{},
				"ref":    "trunk",
			},
			wantErr: false,
		},
		{
			// TODO this test is somewhat silly; it's more of a placeholder in case I decide to handle the API error more elegantly
			name: "input fields, missing required",
			opts: &RunOptions{
				Selector:  "workflow.yml",
				RawFields: []string{`greeting="hello there"`},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/workflow.yml"),
					httpmock.JSONResponse(shared.Workflow{
						Path: ".github/workflows/workflow.yml",
						ID:   12345,
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
					httpmock.StatusStringResponse(422, "missing something"))
			},
			wantErr: true,
			errOut:  "could not create workflow dispatch event: HTTP 422 (https://api.github.com/repos/OWNER/REPO/actions/workflows/12345/dispatches)",
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
			name: "prompt, minimal yaml",
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
								Name:  "minimal workflow",
								ID:    1,
								State: shared.Active,
								Path:  ".github/workflows/minimal.yml",
							},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/minimal.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedNoInputsYAMLContent,
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/1/dispatches"),
					httpmock.StatusStringResponse(204, "cool"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow", []string{"minimal workflow (minimal.yml)"}, func(_, _ string, opts []string) (int, error) {
					return 0, nil
				})
			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{},
				"ref":    "trunk",
			},
			wantOut: "✓ Created workflow_dispatch event for minimal.yml at trunk\n\nTo see runs for this workflow, try: gh run list --workflow=minimal.yml\n",
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
						Content: encodedYAMLContent,
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
					httpmock.StatusStringResponse(204, "cool"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow", []string{"a workflow (workflow.yml)"}, func(_, _ string, opts []string) (int, error) {
					return 0, nil
				})
				pm.RegisterInput("greeting", func(_, _ string) (string, error) {
					return "hi", nil
				})
				pm.RegisterInput("name (required)", func(_, _ string) (string, error) {
					return "scully", nil
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
		{
			name: "prompt, workflow choice input",
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
								Name:  "choice inputs",
								ID:    12345,
								State: shared.Active,
								Path:  ".github/workflows/workflow.yml",
							},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedYAMLContentChoiceIp,
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
					httpmock.StatusStringResponse(204, "cool"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow", []string{"choice inputs (workflow.yml)"}, func(_, _ string, opts []string) (int, error) {
					return 0, nil
				})
				pm.RegisterSelect("favourite-animal (required)", []string{"dog", "cat"}, func(_, _ string, opts []string) (int, error) {
					return 0, nil
				})
				pm.RegisterSelect("name", []string{"monalisa", "cschleiden"}, func(_, _ string, opts []string) (int, error) {
					return 0, nil
				})

			},
			wantBody: map[string]interface{}{
				"inputs": map[string]interface{}{
					"name":             "monalisa",
					"favourite-animal": "dog",
				},
				"ref": "trunk",
			},
			wantOut: "✓ Created workflow_dispatch event for workflow.yml at trunk\n\nTo see runs for this workflow, try: gh run list --workflow=workflow.yml\n",
		},
		{
			name: "prompt, workflow choice missing input",
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
								Name:  "choice missing inputs",
								ID:    12345,
								State: shared.Active,
								Path:  ".github/workflows/workflow.yml",
							},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/.github/workflows/workflow.yml"),
					httpmock.JSONResponse(struct{ Content string }{
						Content: encodedYAMLContentMissingChoiceIp,
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/workflows/12345/dispatches"),
					httpmock.StatusStringResponse(204, "cool"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow", []string{"choice missing inputs (workflow.yml)"}, func(_, _ string, opts []string) (int, error) {
					return 0, nil
				})
				pm.RegisterSelect("name", []string{}, func(_, _ string, opts []string) (int, error) {
					return 0, nil
				})
			},
			wantErr: true,
			errOut:  "workflow input \"name\" is of type choice, but has no options",
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

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return api.InitRepoHostname(&api.Repository{
				Name:             "REPO",
				Owner:            api.RepositoryOwner{Login: "OWNER"},
				DefaultBranchRef: api.BranchRef{Name: "trunk"},
			}, "github.com"), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			pm := prompter.NewMockPrompter(t)
			tt.opts.Prompter = pm
			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

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
					bodyBytes, _ := io.ReadAll(lastRequest.Body)
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
