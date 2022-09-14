package list

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/1"),
					httpmock.JSONResponse(workflowShared.Workflow{
						Name: "CI",
					}))
			},
			wantOut: "STATUS  NAME         WORKFLOW  BRANCH  EVENT  ID    ELAPSED  AGE\nX       cool commit  CI        trunk   push   1     4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   2     4m34s    Feb 23, 2021\n✓       cool commit  CI        trunk   push   3     4m34s    Feb 23, 2021\nX       cool commit  CI        trunk   push   4     4m34s    Feb 23, 2021\nX       cool commit  CI        trunk   push   1234  4m34s    Feb 23, 2021\n-       cool commit  CI        trunk   push   6     4m34s    Feb 23, 2021\n-       cool commit  CI        trunk   push   7     4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   8     4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   9     4m34s    Feb 23, 2021\nX       cool commit  CI        trunk   push   10    4m34s    Feb 23, 2021\n",
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
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/1"),
					httpmock.JSONResponse(workflowShared.Workflow{
						Name: "CI",
					}))
			},
			wantOut: "completed	timed_out	cool commit	CI	trunk	push	1	4m34s	Feb 23, 2021\nin_progress		cool commit	CI	trunk	push	2	4m34s	Feb 23, 2021\ncompleted	success	cool commit	CI	trunk	push	3	4m34s	Feb 23, 2021\ncompleted	cancelled	cool commit	CI	trunk	push	4	4m34s	Feb 23, 2021\ncompleted	failure	cool commit	CI	trunk	push	1234	4m34s	Feb 23, 2021\ncompleted	neutral	cool commit	CI	trunk	push	6	4m34s	Feb 23, 2021\ncompleted	skipped	cool commit	CI	trunk	push	7	4m34s	Feb 23, 2021\nrequested		cool commit	CI	trunk	push	8	4m34s	Feb 23, 2021\nqueued		cool commit	CI	trunk	push	9	4m34s	Feb 23, 2021\ncompleted	stale	cool commit	CI	trunk	push	10	4m34s	Feb 23, 2021\n",
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
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: runs[0:100],
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: runs[100:],
					}))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/1"),
					httpmock.JSONResponse(workflowShared.Workflow{
						Name: "CI",
					}))
			},
			wantOut: longRunOutput,
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
			wantErr: true,
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
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/workflows/1"),
					httpmock.JSONResponse(workflowShared.Workflow{
						Name: "CI",
					}))
			},
			wantOut: "STATUS  NAME         WORKFLOW  BRANCH  EVENT  ID    ELAPSED  AGE\n*       cool commit  CI        trunk   push   2     4m34s    Feb 23, 2021\n✓       cool commit  CI        trunk   push   3     4m34s    Feb 23, 2021\nX       cool commit  CI        trunk   push   1234  4m34s    Feb 23, 2021\n",
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
			wantErr: true,
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
			wantErr: true,
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
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
		})
	}
}

const longRunOutput = "STATUS  NAME         WORKFLOW  BRANCH  EVENT  ID   ELAPSED  AGE\n*       cool commit  CI        trunk   push   0    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   1    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   2    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   3    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   4    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   5    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   6    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   7    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   8    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   9    4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   10   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   11   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   12   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   13   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   14   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   15   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   16   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   17   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   18   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   19   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   20   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   21   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   22   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   23   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   24   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   25   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   26   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   27   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   28   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   29   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   30   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   31   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   32   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   33   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   34   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   35   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   36   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   37   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   38   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   39   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   40   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   41   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   42   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   43   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   44   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   45   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   46   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   47   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   48   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   49   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   50   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   51   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   52   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   53   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   54   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   55   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   56   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   57   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   58   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   59   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   60   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   61   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   62   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   63   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   64   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   65   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   66   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   67   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   68   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   69   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   70   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   71   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   72   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   73   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   74   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   75   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   76   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   77   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   78   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   79   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   80   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   81   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   82   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   83   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   84   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   85   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   86   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   87   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   88   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   89   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   90   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   91   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   92   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   93   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   94   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   95   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   96   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   97   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   98   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   99   4m34s    Feb 23, 2021\n*       cool commit  CI        trunk   push   100  4m34s    Feb 23, 2021\n"
