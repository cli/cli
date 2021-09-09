package list

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

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

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
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

			assert.Equal(t, tt.wants.Limit, gotOpts.Limit)
		})
	}
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ListOptions
		wantOut    string
		wantErrOut string
		stubs      func(*httpmock.Registry)
		nontty     bool
	}{
		{
			name: "blank tty",
			opts: &ListOptions{
				Limit: defaultLimit,
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
			},
			wantOut: "STATUS  NAME         WORKFLOW     BRANCH  EVENT  ID    ELAPSED  AGE\nX       cool commit  timed out    trunk   push   1     4m34s    Feb 23, 2021\n-       cool commit  in progress  trunk   push   2     4m34s    Feb 23, 2021\n✓       cool commit  successful   trunk   push   3     4m34s    Feb 23, 2021\n✓       cool commit  cancelled    trunk   push   4     4m34s    Feb 23, 2021\nX       cool commit  failed       trunk   push   1234  4m34s    Feb 23, 2021\n✓       cool commit  neutral      trunk   push   6     4m34s    Feb 23, 2021\n✓       cool commit  skipped      trunk   push   7     4m34s    Feb 23, 2021\n-       cool commit  requested    trunk   push   8     4m34s    Feb 23, 2021\n-       cool commit  queued       trunk   push   9     4m34s    Feb 23, 2021\nX       cool commit  stale        trunk   push   10    4m34s    Feb 23, 2021\n\nFor details on a run, try: gh run view <run-id>\n",
		},
		{
			name: "blank nontty",
			opts: &ListOptions{
				Limit:       defaultLimit,
				PlainOutput: true,
			},
			nontty: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{
						WorkflowRuns: shared.TestRuns,
					}))
			},
			wantOut: "completed\ttimed_out\tcool commit\ttimed out\ttrunk\tpush\t1\t4m34s\tFeb 23, 2021\nin_progress\t\tcool commit\tin progress\ttrunk\tpush\t2\t4m34s\tFeb 23, 2021\ncompleted\tsuccess\tcool commit\tsuccessful\ttrunk\tpush\t3\t4m34s\tFeb 23, 2021\ncompleted\tcancelled\tcool commit\tcancelled\ttrunk\tpush\t4\t4m34s\tFeb 23, 2021\ncompleted\tfailure\tcool commit\tfailed\ttrunk\tpush\t1234\t4m34s\tFeb 23, 2021\ncompleted\tneutral\tcool commit\tneutral\ttrunk\tpush\t6\t4m34s\tFeb 23, 2021\ncompleted\tskipped\tcool commit\tskipped\ttrunk\tpush\t7\t4m34s\tFeb 23, 2021\nrequested\t\tcool commit\trequested\ttrunk\tpush\t8\t4m34s\tFeb 23, 2021\nqueued\t\tcool commit\tqueued\ttrunk\tpush\t9\t4m34s\tFeb 23, 2021\ncompleted\tstale\tcool commit\tstale\ttrunk\tpush\t10\t4m34s\tFeb 23, 2021\n",
		},
		{
			name: "pagination",
			opts: &ListOptions{
				Limit: 101,
			},
			stubs: func(reg *httpmock.Registry) {
				var runID int64
				runs := []shared.Run{}
				for runID < 103 {
					runs = append(runs, shared.TestRun(fmt.Sprintf("%d", runID), runID, shared.InProgress, ""))
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
			},
			wantOut: longRunOutput,
		},
		{
			name: "no results nontty",
			opts: &ListOptions{
				Limit:       defaultLimit,
				PlainOutput: true,
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			nontty:  true,
			wantOut: "",
		},
		{
			name: "no results tty",
			opts: &ListOptions{
				Limit: defaultLimit,
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/runs"),
					httpmock.JSONResponse(shared.RunsPayload{}),
				)
			},
			wantOut:    "",
			wantErrOut: "No runs found\n",
		},
		{
			name: "workflow selector",
			opts: &ListOptions{
				Limit:            defaultLimit,
				WorkflowSelector: "flow.yml",
			},
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
			wantOut: "STATUS  NAME         WORKFLOW     BRANCH  EVENT  ID    ELAPSED  AGE\n-       cool commit  in progress  trunk   push   2     4m34s    Feb 23, 2021\n✓       cool commit  successful   trunk   push   3     4m34s    Feb 23, 2021\nX       cool commit  failed       trunk   push   1234  4m34s    Feb 23, 2021\n\nFor details on a run, try: gh run view <run-id>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			tt.stubs(reg)

			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(!tt.nontty)
			tt.opts.IO = io
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := listRun(tt.opts)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantErrOut, stderr.String())
			reg.Verify(t)
		})
	}
}

const longRunOutput = "STATUS  NAME         WORKFLOW  BRANCH  EVENT  ID   ELAPSED  AGE\n-       cool commit  0         trunk   push   0    4m34s    Feb 23, 2021\n-       cool commit  1         trunk   push   1    4m34s    Feb 23, 2021\n-       cool commit  2         trunk   push   2    4m34s    Feb 23, 2021\n-       cool commit  3         trunk   push   3    4m34s    Feb 23, 2021\n-       cool commit  4         trunk   push   4    4m34s    Feb 23, 2021\n-       cool commit  5         trunk   push   5    4m34s    Feb 23, 2021\n-       cool commit  6         trunk   push   6    4m34s    Feb 23, 2021\n-       cool commit  7         trunk   push   7    4m34s    Feb 23, 2021\n-       cool commit  8         trunk   push   8    4m34s    Feb 23, 2021\n-       cool commit  9         trunk   push   9    4m34s    Feb 23, 2021\n-       cool commit  10        trunk   push   10   4m34s    Feb 23, 2021\n-       cool commit  11        trunk   push   11   4m34s    Feb 23, 2021\n-       cool commit  12        trunk   push   12   4m34s    Feb 23, 2021\n-       cool commit  13        trunk   push   13   4m34s    Feb 23, 2021\n-       cool commit  14        trunk   push   14   4m34s    Feb 23, 2021\n-       cool commit  15        trunk   push   15   4m34s    Feb 23, 2021\n-       cool commit  16        trunk   push   16   4m34s    Feb 23, 2021\n-       cool commit  17        trunk   push   17   4m34s    Feb 23, 2021\n-       cool commit  18        trunk   push   18   4m34s    Feb 23, 2021\n-       cool commit  19        trunk   push   19   4m34s    Feb 23, 2021\n-       cool commit  20        trunk   push   20   4m34s    Feb 23, 2021\n-       cool commit  21        trunk   push   21   4m34s    Feb 23, 2021\n-       cool commit  22        trunk   push   22   4m34s    Feb 23, 2021\n-       cool commit  23        trunk   push   23   4m34s    Feb 23, 2021\n-       cool commit  24        trunk   push   24   4m34s    Feb 23, 2021\n-       cool commit  25        trunk   push   25   4m34s    Feb 23, 2021\n-       cool commit  26        trunk   push   26   4m34s    Feb 23, 2021\n-       cool commit  27        trunk   push   27   4m34s    Feb 23, 2021\n-       cool commit  28        trunk   push   28   4m34s    Feb 23, 2021\n-       cool commit  29        trunk   push   29   4m34s    Feb 23, 2021\n-       cool commit  30        trunk   push   30   4m34s    Feb 23, 2021\n-       cool commit  31        trunk   push   31   4m34s    Feb 23, 2021\n-       cool commit  32        trunk   push   32   4m34s    Feb 23, 2021\n-       cool commit  33        trunk   push   33   4m34s    Feb 23, 2021\n-       cool commit  34        trunk   push   34   4m34s    Feb 23, 2021\n-       cool commit  35        trunk   push   35   4m34s    Feb 23, 2021\n-       cool commit  36        trunk   push   36   4m34s    Feb 23, 2021\n-       cool commit  37        trunk   push   37   4m34s    Feb 23, 2021\n-       cool commit  38        trunk   push   38   4m34s    Feb 23, 2021\n-       cool commit  39        trunk   push   39   4m34s    Feb 23, 2021\n-       cool commit  40        trunk   push   40   4m34s    Feb 23, 2021\n-       cool commit  41        trunk   push   41   4m34s    Feb 23, 2021\n-       cool commit  42        trunk   push   42   4m34s    Feb 23, 2021\n-       cool commit  43        trunk   push   43   4m34s    Feb 23, 2021\n-       cool commit  44        trunk   push   44   4m34s    Feb 23, 2021\n-       cool commit  45        trunk   push   45   4m34s    Feb 23, 2021\n-       cool commit  46        trunk   push   46   4m34s    Feb 23, 2021\n-       cool commit  47        trunk   push   47   4m34s    Feb 23, 2021\n-       cool commit  48        trunk   push   48   4m34s    Feb 23, 2021\n-       cool commit  49        trunk   push   49   4m34s    Feb 23, 2021\n-       cool commit  50        trunk   push   50   4m34s    Feb 23, 2021\n-       cool commit  51        trunk   push   51   4m34s    Feb 23, 2021\n-       cool commit  52        trunk   push   52   4m34s    Feb 23, 2021\n-       cool commit  53        trunk   push   53   4m34s    Feb 23, 2021\n-       cool commit  54        trunk   push   54   4m34s    Feb 23, 2021\n-       cool commit  55        trunk   push   55   4m34s    Feb 23, 2021\n-       cool commit  56        trunk   push   56   4m34s    Feb 23, 2021\n-       cool commit  57        trunk   push   57   4m34s    Feb 23, 2021\n-       cool commit  58        trunk   push   58   4m34s    Feb 23, 2021\n-       cool commit  59        trunk   push   59   4m34s    Feb 23, 2021\n-       cool commit  60        trunk   push   60   4m34s    Feb 23, 2021\n-       cool commit  61        trunk   push   61   4m34s    Feb 23, 2021\n-       cool commit  62        trunk   push   62   4m34s    Feb 23, 2021\n-       cool commit  63        trunk   push   63   4m34s    Feb 23, 2021\n-       cool commit  64        trunk   push   64   4m34s    Feb 23, 2021\n-       cool commit  65        trunk   push   65   4m34s    Feb 23, 2021\n-       cool commit  66        trunk   push   66   4m34s    Feb 23, 2021\n-       cool commit  67        trunk   push   67   4m34s    Feb 23, 2021\n-       cool commit  68        trunk   push   68   4m34s    Feb 23, 2021\n-       cool commit  69        trunk   push   69   4m34s    Feb 23, 2021\n-       cool commit  70        trunk   push   70   4m34s    Feb 23, 2021\n-       cool commit  71        trunk   push   71   4m34s    Feb 23, 2021\n-       cool commit  72        trunk   push   72   4m34s    Feb 23, 2021\n-       cool commit  73        trunk   push   73   4m34s    Feb 23, 2021\n-       cool commit  74        trunk   push   74   4m34s    Feb 23, 2021\n-       cool commit  75        trunk   push   75   4m34s    Feb 23, 2021\n-       cool commit  76        trunk   push   76   4m34s    Feb 23, 2021\n-       cool commit  77        trunk   push   77   4m34s    Feb 23, 2021\n-       cool commit  78        trunk   push   78   4m34s    Feb 23, 2021\n-       cool commit  79        trunk   push   79   4m34s    Feb 23, 2021\n-       cool commit  80        trunk   push   80   4m34s    Feb 23, 2021\n-       cool commit  81        trunk   push   81   4m34s    Feb 23, 2021\n-       cool commit  82        trunk   push   82   4m34s    Feb 23, 2021\n-       cool commit  83        trunk   push   83   4m34s    Feb 23, 2021\n-       cool commit  84        trunk   push   84   4m34s    Feb 23, 2021\n-       cool commit  85        trunk   push   85   4m34s    Feb 23, 2021\n-       cool commit  86        trunk   push   86   4m34s    Feb 23, 2021\n-       cool commit  87        trunk   push   87   4m34s    Feb 23, 2021\n-       cool commit  88        trunk   push   88   4m34s    Feb 23, 2021\n-       cool commit  89        trunk   push   89   4m34s    Feb 23, 2021\n-       cool commit  90        trunk   push   90   4m34s    Feb 23, 2021\n-       cool commit  91        trunk   push   91   4m34s    Feb 23, 2021\n-       cool commit  92        trunk   push   92   4m34s    Feb 23, 2021\n-       cool commit  93        trunk   push   93   4m34s    Feb 23, 2021\n-       cool commit  94        trunk   push   94   4m34s    Feb 23, 2021\n-       cool commit  95        trunk   push   95   4m34s    Feb 23, 2021\n-       cool commit  96        trunk   push   96   4m34s    Feb 23, 2021\n-       cool commit  97        trunk   push   97   4m34s    Feb 23, 2021\n-       cool commit  98        trunk   push   98   4m34s    Feb 23, 2021\n-       cool commit  99        trunk   push   99   4m34s    Feb 23, 2021\n-       cool commit  100       trunk   push   100  4m34s    Feb 23, 2021\n\nFor details on a run, try: gh run view <run-id>\n"
