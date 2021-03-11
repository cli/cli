package list

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/run/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
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
			wantOut: "X  cool commit  timed out    trunk  push  1\n-  cool commit  in progress  trunk  push  2\n✓  cool commit  successful   trunk  push  3\n✓  cool commit  cancelled    trunk  push  4\nX  cool commit  failed       trunk  push  1234\n✓  cool commit  neutral      trunk  push  6\n✓  cool commit  skipped      trunk  push  7\n-  cool commit  requested    trunk  push  8\n-  cool commit  queued       trunk  push  9\nX  cool commit  stale        trunk  push  10\n\nFor details on a run, try: gh run view <run-id>\n",
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
			wantOut: "completed\ttimed_out\tcool commit\ttimed out\ttrunk\tpush\t4m34s\t1\nin_progress\t\tcool commit\tin progress\ttrunk\tpush\t4m34s\t2\ncompleted\tsuccess\tcool commit\tsuccessful\ttrunk\tpush\t4m34s\t3\ncompleted\tcancelled\tcool commit\tcancelled\ttrunk\tpush\t4m34s\t4\ncompleted\tfailure\tcool commit\tfailed\ttrunk\tpush\t4m34s\t1234\ncompleted\tneutral\tcool commit\tneutral\ttrunk\tpush\t4m34s\t6\ncompleted\tskipped\tcool commit\tskipped\ttrunk\tpush\t4m34s\t7\nrequested\t\tcool commit\trequested\ttrunk\tpush\t4m34s\t8\nqueued\t\tcool commit\tqueued\ttrunk\tpush\t4m34s\t9\ncompleted\tstale\tcool commit\tstale\ttrunk\tpush\t4m34s\t10\n",
		},
		{
			name: "pagination",
			opts: &ListOptions{
				Limit: 101,
			},
			stubs: func(reg *httpmock.Registry) {
				runID := 0
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

const longRunOutput = "-  cool commit  0    trunk  push  0\n-  cool commit  1    trunk  push  1\n-  cool commit  2    trunk  push  2\n-  cool commit  3    trunk  push  3\n-  cool commit  4    trunk  push  4\n-  cool commit  5    trunk  push  5\n-  cool commit  6    trunk  push  6\n-  cool commit  7    trunk  push  7\n-  cool commit  8    trunk  push  8\n-  cool commit  9    trunk  push  9\n-  cool commit  10   trunk  push  10\n-  cool commit  11   trunk  push  11\n-  cool commit  12   trunk  push  12\n-  cool commit  13   trunk  push  13\n-  cool commit  14   trunk  push  14\n-  cool commit  15   trunk  push  15\n-  cool commit  16   trunk  push  16\n-  cool commit  17   trunk  push  17\n-  cool commit  18   trunk  push  18\n-  cool commit  19   trunk  push  19\n-  cool commit  20   trunk  push  20\n-  cool commit  21   trunk  push  21\n-  cool commit  22   trunk  push  22\n-  cool commit  23   trunk  push  23\n-  cool commit  24   trunk  push  24\n-  cool commit  25   trunk  push  25\n-  cool commit  26   trunk  push  26\n-  cool commit  27   trunk  push  27\n-  cool commit  28   trunk  push  28\n-  cool commit  29   trunk  push  29\n-  cool commit  30   trunk  push  30\n-  cool commit  31   trunk  push  31\n-  cool commit  32   trunk  push  32\n-  cool commit  33   trunk  push  33\n-  cool commit  34   trunk  push  34\n-  cool commit  35   trunk  push  35\n-  cool commit  36   trunk  push  36\n-  cool commit  37   trunk  push  37\n-  cool commit  38   trunk  push  38\n-  cool commit  39   trunk  push  39\n-  cool commit  40   trunk  push  40\n-  cool commit  41   trunk  push  41\n-  cool commit  42   trunk  push  42\n-  cool commit  43   trunk  push  43\n-  cool commit  44   trunk  push  44\n-  cool commit  45   trunk  push  45\n-  cool commit  46   trunk  push  46\n-  cool commit  47   trunk  push  47\n-  cool commit  48   trunk  push  48\n-  cool commit  49   trunk  push  49\n-  cool commit  50   trunk  push  50\n-  cool commit  51   trunk  push  51\n-  cool commit  52   trunk  push  52\n-  cool commit  53   trunk  push  53\n-  cool commit  54   trunk  push  54\n-  cool commit  55   trunk  push  55\n-  cool commit  56   trunk  push  56\n-  cool commit  57   trunk  push  57\n-  cool commit  58   trunk  push  58\n-  cool commit  59   trunk  push  59\n-  cool commit  60   trunk  push  60\n-  cool commit  61   trunk  push  61\n-  cool commit  62   trunk  push  62\n-  cool commit  63   trunk  push  63\n-  cool commit  64   trunk  push  64\n-  cool commit  65   trunk  push  65\n-  cool commit  66   trunk  push  66\n-  cool commit  67   trunk  push  67\n-  cool commit  68   trunk  push  68\n-  cool commit  69   trunk  push  69\n-  cool commit  70   trunk  push  70\n-  cool commit  71   trunk  push  71\n-  cool commit  72   trunk  push  72\n-  cool commit  73   trunk  push  73\n-  cool commit  74   trunk  push  74\n-  cool commit  75   trunk  push  75\n-  cool commit  76   trunk  push  76\n-  cool commit  77   trunk  push  77\n-  cool commit  78   trunk  push  78\n-  cool commit  79   trunk  push  79\n-  cool commit  80   trunk  push  80\n-  cool commit  81   trunk  push  81\n-  cool commit  82   trunk  push  82\n-  cool commit  83   trunk  push  83\n-  cool commit  84   trunk  push  84\n-  cool commit  85   trunk  push  85\n-  cool commit  86   trunk  push  86\n-  cool commit  87   trunk  push  87\n-  cool commit  88   trunk  push  88\n-  cool commit  89   trunk  push  89\n-  cool commit  90   trunk  push  90\n-  cool commit  91   trunk  push  91\n-  cool commit  92   trunk  push  92\n-  cool commit  93   trunk  push  93\n-  cool commit  94   trunk  push  94\n-  cool commit  95   trunk  push  95\n-  cool commit  96   trunk  push  96\n-  cool commit  97   trunk  push  97\n-  cool commit  98   trunk  push  98\n-  cool commit  99   trunk  push  99\n-  cool commit  100  trunk  push  100\n\nFor details on a run, try: gh run view <run-id>\n"
