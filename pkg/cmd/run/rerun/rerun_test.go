package rerun

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
		{
			name: "failed arg nontty",
			cli:  "4321 --failed",
			wants: RerunOptions{
				RunID:      "4321",
				OnlyFailed: true,
			},
		},
		{
			name: "failed arg",
			tty:  true,
			cli:  "--failed",
			wants: RerunOptions{
				Prompt:     true,
				OnlyFailed: true,
			},
		},
		{
			name: "with arg job",
			tty:  true,
			cli:  "--job 1234",
			wants: RerunOptions{
				JobID: "1234",
			},
		},
		{
			name: "with args jobID and runID uses jobID",
			tty:  true,
			cli:  "1234 --job 5678",
			wants: RerunOptions{
				JobID: "5678",
				RunID: "",
			},
		},
		{
			name:     "with arg job with no ID fails",
			tty:      true,
			cli:      "--job",
			wantsErr: true,
		},
		{
			name:     "with arg job with no ID no tty fails",
			cli:      "--job",
			wantsErr: true,
		},
		{
			name: "debug nontty",
			cli:  "4321 --debug",
			wants: RerunOptions{
				RunID: "4321",
				Debug: true,
			},
		},
		{
			name: "debug tty",
			tty:  true,
			cli:  "--debug",
			wants: RerunOptions{
				Prompt: true,
				Debug:  true,
			},
		},
		{
			name: "debug off",
			cli:  "4321 --debug=false",
			wants: RerunOptions{
				RunID: "4321",
				Debug: false,
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

			var gotOpts *RerunOptions
			cmd := NewCmdRerun(f, func(opts *RerunOptions) error {
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

			assert.Equal(t, tt.wants.RunID, gotOpts.RunID)
			assert.Equal(t, tt.wants.Prompt, gotOpts.Prompt)
		})
	}

}

func TestRerun(t *testing.T) {
	tests := []struct {
		name        string
		httpStubs   func(*httpmock.Registry)
		promptStubs func(*prompter.MockPrompter)
		opts        *RerunOptions
		tty         bool
		wantErr     bool
		errOut      string
		wantOut     string
		wantDebug   bool
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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/rerun"),
					httpmock.StringResponse("{}"))
			},
			wantOut: "✓ Requested rerun of run 1234\n",
		},
		{
			name: "arg including onlyFailed",
			tty:  true,
			opts: &RerunOptions{
				RunID:      "1234",
				OnlyFailed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/rerun-failed-jobs"),
					httpmock.StringResponse("{}"))
			},
			wantOut: "✓ Requested rerun (failed jobs) of run 1234\n",
		},
		{
			name: "arg including a specific job",
			tty:  true,
			opts: &RerunOptions{
				JobID: "20", // 20 is shared.FailedJob.ID
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20"),
					httpmock.JSONResponse(shared.FailedJob))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/jobs/20/rerun"),
					httpmock.StringResponse("{}"))
			},
			wantOut: "✓ Requested rerun of job 20 on run 1234\n",
		},
		{
			name: "arg including debug",
			tty:  true,
			opts: &RerunOptions{
				RunID: "1234",
				Debug: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/rerun"),
					httpmock.StringResponse("{}"))
			},
			wantOut:   "✓ Requested rerun of run 1234 with debug logging enabled\n",
			wantDebug: true,
		},
		{
			name: "arg including onlyFailed and debug",
			tty:  true,
			opts: &RerunOptions{
				RunID:      "1234",
				OnlyFailed: true,
				Debug:      true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs/1234"),
					httpmock.JSONResponse(shared.FailedRun))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/rerun-failed-jobs"),
					httpmock.StringResponse("{}"))
			},
			wantOut:   "✓ Requested rerun (failed jobs) of run 1234 with debug logging enabled\n",
			wantDebug: true,
		},
		{
			name: "arg including a specific job and debug",
			tty:  true,
			opts: &RerunOptions{
				JobID: "20", // 20 is shared.FailedJob.ID
				Debug: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/20"),
					httpmock.JSONResponse(shared.FailedJob))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/jobs/20/rerun"),
					httpmock.StringResponse("{}"))
			},
			wantOut:   "✓ Requested rerun of job 20 on run 1234 with debug logging enabled\n",
			wantDebug: true,
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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/1234/rerun"),
					httpmock.StringResponse("{}"))
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Select a workflow run",
					[]string{"X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "- cool commit, CI [trunk] Feb 23, 2021", "X cool commit, CI [trunk] Feb 23, 2021"},
					func(_, _ string, opts []string) (int, error) {
						return 2, nil
					})
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
							shared.TestRun(2, shared.InProgress, ""),
						}}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			wantErr: true,
			errOut:  "no recent runs have failed; please specify a specific `<run-id>`",
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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/actions/runs/3/rerun"),
					httpmock.StatusStringResponse(403, "no"))
			},
			wantErr: true,
			errOut:  "run 3 cannot be rerun; its workflow file may be broken",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("OWNER/REPO")
		}

		pm := prompter.NewMockPrompter(t)
		tt.opts.Prompter = pm
		if tt.promptStubs != nil {
			tt.promptStubs(pm)
		}

		t.Run(tt.name, func(t *testing.T) {
			err := runRerun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errOut, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)

			for _, d := range reg.Requests {
				if d.Method != "POST" {
					continue
				}

				if !tt.wantDebug {
					assert.Nil(t, d.Body)
					continue
				}

				data, err := io.ReadAll(d.Body)
				assert.NoError(t, err)
				var payload RerunPayload
				err = json.Unmarshal(data, &payload)
				assert.NoError(t, err)
				assert.Equal(t, tt.wantDebug, payload.Debug)
			}
		})
	}
}
