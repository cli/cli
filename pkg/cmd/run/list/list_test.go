package list

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	workflowShared "github.com/cli/cli/v2/pkg/cmd/workflow/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		tty      bool
		wants    ListOptions
		wantsErr bool
	}{
		{
			name: "blank",
			wants: ListOptions{
				Limit: defaultLimit,
			},
		},
		{
			name: "limit",
			cli:  "--limit 100",
			wants: ListOptions{
				Limit: 100,
			},
		},
		{
			name:     "bad limit",
			cli:      "--limit hi",
			wantsErr: true,
		},
		{
			name: "workflow",
			cli:  "--workflow foo.yml",
			wants: ListOptions{
				Limit:            defaultLimit,
				WorkflowSelector: "foo.yml",
			},
		},
		{
			name: "branch",
			cli:  "--branch new-cool-stuff",
			wants: ListOptions{
				Limit:  defaultLimit,
				Branch: "new-cool-stuff",
			},
		},
		{
			name: "user",
			cli:  "--user bak1an",
			wants: ListOptions{
				Limit: defaultLimit,
				Actor: "bak1an",
			},
		},
		{
			name: "status",
			cli:  "--status completed",
			wants: ListOptions{
				Limit:  defaultLimit,
				Status: "completed",
			},
		},
		{
			name: "event",
			cli:  "--event push",
			wants: ListOptions{
				Limit: defaultLimit,
				Event: "push",
			},
		},
		{
			name: "created",
			cli:  "--created >=2023-04-24",
			wants: ListOptions{
				Limit:   defaultLimit,
				Created: ">=2023-04-24",
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

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
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

			assert.Equal(t, tt.wants.Limit, gotOpts.Limit)
			assert.Equal(t, tt.wants.WorkflowSelector, gotOpts.WorkflowSelector)
			assert.Equal(t, tt.wants.Branch, gotOpts.Branch)
			assert.Equal(t, tt.wants.Actor, gotOpts.Actor)
			assert.Equal(t, tt.wants.Status, gotOpts.Status)
			assert.Equal(t, tt.wants.Event, gotOpts.Event)
			assert.Equal(t, tt.wants.Created, gotOpts.Created)
		})
	}
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ListOptions
		wantErr    bool
		wantOut    string
		wantErrOut string
		wantErrMsg string
		stubs      func(*httpmock.Registry)
		isTTY      bool
	}{
		{
			name: "default arguments",
			opts: &ListOptions{
				Limit: defaultLimit,
				now:   shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			wantOut: heredoc.Doc(`
				STATUS  TITLE        WORKFLOW  BRANCH  EVENT  ID    ELAPSED  AGE
				X       cool commit  CI        trunk   push   1     4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   2     4m34s    about 4 minutes ago
				✓       cool commit  CI        trunk   push   3     4m34s    about 4 minutes ago
				X       cool commit  CI        trunk   push   4     4m34s    about 4 minutes ago
				X       cool commit  CI        trunk   push   1234  4m34s    about 4 minutes ago
				-       cool commit  CI        trunk   push   6     4m34s    about 4 minutes ago
				-       cool commit  CI        trunk   push   7     4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   8     4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   9     4m34s    about 4 minutes ago
				X       cool commit  CI        trunk   push   10    4m34s    about 4 minutes ago
			`),
		},
		{
			name: "inactive disabled workflow selected",
			opts: &ListOptions{
				Limit:            defaultLimit,
				now:              shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
				WorkflowSelector: "d. inact",
				All:              false,
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				// Uses abbreviated names and commit messages because of output column limit
				workflow := workflowShared.Workflow{
					Name:  "d. inact",
					ID:    1206,
					Path:  ".github/workflows/disabledInactivity.yml",
					State: workflowShared.DisabledInactivity,
				}

				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							workflow,
						},
					}))
			},
			wantErr:    true,
			wantErrMsg: "could not find any workflows named d. inact",
		},
		{
			name: "inactive disabled workflow selected and all states applied",
			opts: &ListOptions{
				Limit:            defaultLimit,
				now:              shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
				WorkflowSelector: "d. inact",
				All:              true,
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				// Uses abbreviated names and commit messages because of output column limit
				workflow := workflowShared.Workflow{
					Name:  "d. inact",
					ID:    1206,
					Path:  ".github/workflows/disabledInactivity.yml",
					State: workflowShared.DisabledInactivity,
				}

				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							workflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/actions/workflows/%d/runs", workflow.ID)),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: []shared.Run{
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 101, shared.Completed, shared.TimedOut, "dicto"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 102, shared.InProgress, shared.TimedOut, "diito"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 103, shared.Completed, shared.Success, "dics"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 104, shared.Completed, shared.Cancelled, "dicc"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 105, shared.Completed, shared.Failure, "dicf"),
						},
					}))
			},
			wantOut: heredoc.Doc(`
				STATUS  TITLE  WORKFLOW  BRANCH  EVENT  ID   ELAPSED  AGE
				X       dicto  d. inact  trunk   push   101  4m34s    about 4 minutes ago
				*       diito  d. inact  trunk   push   102  4m34s    about 4 minutes ago
				✓       dics   d. inact  trunk   push   103  4m34s    about 4 minutes ago
				X       dicc   d. inact  trunk   push   104  4m34s    about 4 minutes ago
				X       dicf   d. inact  trunk   push   105  4m34s    about 4 minutes ago
			`),
		},
		{
			name: "manually disabled workflow selected",
			opts: &ListOptions{
				Limit:            defaultLimit,
				now:              shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
				WorkflowSelector: "d. man",
				All:              false,
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				// Uses abbreviated names and commit messages because of output column limit
				workflow := workflowShared.Workflow{
					Name:  "d. man",
					ID:    456,
					Path:  ".github/workflows/disabled.yml",
					State: workflowShared.DisabledManually,
				}

				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							workflow,
						},
					}))
			},
			wantErr:    true,
			wantErrMsg: "could not find any workflows named d. man",
		},
		{
			name: "manually disabled workflow selected and all states applied",
			opts: &ListOptions{
				Limit:            defaultLimit,
				now:              shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
				WorkflowSelector: "d. man",
				All:              true,
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				// Uses abbreviated names and commit messages because of output column limit
				workflow := workflowShared.Workflow{
					Name:  "d. man",
					ID:    456,
					Path:  ".github/workflows/disabled.yml",
					State: workflowShared.DisabledManually,
				}

				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							workflow,
						},
					}))
				reg.Register(
					httpmock.REST("GET", fmt.Sprintf("repos/OWNER/REPO/actions/workflows/%d/runs", workflow.ID)),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: []shared.Run{
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 201, shared.Completed, shared.TimedOut, "dmcto"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 202, shared.InProgress, shared.TimedOut, "dmito"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 203, shared.Completed, shared.Success, "dmcs"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 204, shared.Completed, shared.Cancelled, "dmcc"),
							shared.TestRunWithWorkflowAndCommit(workflow.ID, 205, shared.Completed, shared.Failure, "dmcf"),
						},
					}))
			},
			wantOut: heredoc.Doc(`
				STATUS  TITLE  WORKFLOW  BRANCH  EVENT  ID   ELAPSED  AGE
				X       dmcto  d. man    trunk   push   201  4m34s    about 4 minutes ago
				*       dmito  d. man    trunk   push   202  4m34s    about 4 minutes ago
				✓       dmcs   d. man    trunk   push   203  4m34s    about 4 minutes ago
				X       dmcc   d. man    trunk   push   204  4m34s    about 4 minutes ago
				X       dmcf   d. man    trunk   push   205  4m34s    about 4 minutes ago
			`),
		},
		{
			name: "default arguments nontty",
			opts: &ListOptions{
				Limit: defaultLimit,
				now:   shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
			},
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			wantOut: heredoc.Doc(`
				completed	timed_out	cool commit	CI	trunk	push	1	4m34s	2021-02-23T04:51:00Z
				in_progress		cool commit	CI	trunk	push	2	4m34s	2021-02-23T04:51:00Z
				completed	success	cool commit	CI	trunk	push	3	4m34s	2021-02-23T04:51:00Z
				completed	cancelled	cool commit	CI	trunk	push	4	4m34s	2021-02-23T04:51:00Z
				completed	failure	cool commit	CI	trunk	push	1234	4m34s	2021-02-23T04:51:00Z
				completed	neutral	cool commit	CI	trunk	push	6	4m34s	2021-02-23T04:51:00Z
				completed	skipped	cool commit	CI	trunk	push	7	4m34s	2021-02-23T04:51:00Z
				requested		cool commit	CI	trunk	push	8	4m34s	2021-02-23T04:51:00Z
				queued		cool commit	CI	trunk	push	9	4m34s	2021-02-23T04:51:00Z
				completed	stale	cool commit	CI	trunk	push	10	4m34s	2021-02-23T04:51:00Z
			`),
		},
		{
			name: "pagination",
			opts: &ListOptions{
				Limit: 101,
				now:   shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				var runID int64
				runs := []shared.Run{}
				for runID < 103 {
					runs = append(runs, shared.TestRun(runID, shared.InProgress, ""))
					runID++
				}
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.WithHeader(httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: runs[0:100],
					}), "Link", `<https://api.github.com/repositories/123/actions/runs?per_page=100&page=2>; rel="next"`))
				reg.Register(
					httpmock.REST("GET", "repositories/123/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: runs[100:],
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows"),
					httpmock.JSONResponse(workflowShared.WorkflowsPayload{
						Workflows: []workflowShared.Workflow{
							shared.TestWorkflow,
						},
					}))
			},
			wantOut: heredoc.Doc(`
				STATUS  TITLE        WORKFLOW  BRANCH  EVENT  ID   ELAPSED  AGE
				*       cool commit  CI        trunk   push   0    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   1    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   2    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   3    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   4    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   5    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   6    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   7    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   8    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   9    4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   10   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   11   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   12   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   13   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   14   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   15   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   16   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   17   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   18   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   19   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   20   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   21   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   22   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   23   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   24   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   25   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   26   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   27   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   28   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   29   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   30   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   31   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   32   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   33   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   34   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   35   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   36   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   37   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   38   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   39   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   40   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   41   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   42   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   43   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   44   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   45   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   46   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   47   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   48   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   49   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   50   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   51   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   52   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   53   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   54   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   55   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   56   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   57   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   58   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   59   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   60   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   61   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   62   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   63   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   64   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   65   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   66   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   67   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   68   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   69   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   70   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   71   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   72   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   73   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   74   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   75   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   76   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   77   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   78   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   79   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   80   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   81   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   82   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   83   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   84   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   85   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   86   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   87   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   88   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   89   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   90   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   91   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   92   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   93   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   94   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   95   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   96   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   97   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   98   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   99   4m34s    about 4 minutes ago
				*       cool commit  CI        trunk   push   100  4m34s    about 4 minutes ago
			`),
		},
		{
			name: "no results",
			opts: &ListOptions{
				Limit: defaultLimit,
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantErr:    true,
			wantErrMsg: "no runs found",
		},
		{
			name: "workflow selector",
			opts: &ListOptions{
				Limit:            defaultLimit,
				WorkflowSelector: "flow.yml",
				now:              shared.TestRunStartTime.Add(time.Minute*4 + time.Second*34),
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/flow.yml"),
					httpmock.JSONResponse(workflowShared.AWorkflow))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/123/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.WorkflowRuns,
					}))
			},
			wantOut: heredoc.Doc(`
				STATUS  TITLE        WORKFLOW    BRANCH  EVENT  ID    ELAPSED  AGE
				*       cool commit  a workflow  trunk   push   2     4m34s    about 4 minute...
				✓       cool commit  a workflow  trunk   push   3     4m34s    about 4 minute...
				X       cool commit  a workflow  trunk   push   1234  4m34s    about 4 minute...
			`),
		},
		{
			name: "branch filter applied",
			opts: &ListOptions{
				Limit:  defaultLimit,
				Branch: "the-branch",
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/actions/runs", url.Values{
						"branch": []string{"the-branch"},
					}),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantErr:    true,
			wantErrMsg: "no runs found",
		},
		{
			name: "actor filter applied",
			opts: &ListOptions{
				Limit: defaultLimit,
				Actor: "bak1an",
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/actions/runs", url.Values{
						"actor": []string{"bak1an"},
					}),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantErr:    true,
			wantErrMsg: "no runs found",
		},
		{
			name: "status filter applied",
			opts: &ListOptions{
				Limit:  defaultLimit,
				Status: "queued",
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/actions/runs", url.Values{
						"status": []string{"queued"},
					}),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantErr:    true,
			wantErrMsg: "no runs found",
		},
		{
			name: "event filter applied",
			opts: &ListOptions{
				Limit: defaultLimit,
				Event: "push",
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/actions/runs", url.Values{
						"event": []string{"push"},
					}),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantErr:    true,
			wantErrMsg: "no runs found",
		},
		{
			name: "created filter applied",
			opts: &ListOptions{
				Limit:   defaultLimit,
				Created: ">=2023-04-24",
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/actions/runs", url.Values{
						"created": []string{">=2023-04-24"},
					}),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantErr:    true,
			wantErrMsg: "no runs found",
		},
		{
			name: "commit filter applied",
			opts: &ListOptions{
				Limit:  defaultLimit,
				Commit: "1234567890123456789012345678901234567890",
			},
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("GET", "repos/OWNER/REPO/actions/runs", url.Values{
						"head_sha": []string{"1234567890123456789012345678901234567890"},
					}),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantErr:    true,
			wantErrMsg: "no runs found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)

			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			tt.opts.IO = ios
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := listRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
		})
	}
}
