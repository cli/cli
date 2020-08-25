package checks

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdChecks(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants ChecksOptions
	}{
		{
			name:  "no arguments",
			cli:   "",
			wants: ChecksOptions{},
		},
		{
			name: "pr argument",
			cli:  "1234",
			wants: ChecksOptions{
				SelectorArg: "1234",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ChecksOptions
			cmd := NewCmdChecks(f, func(opts *ChecksOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.SelectorArg, gotOpts.SelectorArg)
		})
	}
}

func completedAt() time.Time {
	t, _ := time.Parse("2006-Jan-02", "2020-Jun-15")
	return t
}

func startedAt() time.Time {
	t, _ := time.Parse("2006-Jan-02", "2020-Jun-14")
	return t
}

func Test_checksRun_tty(t *testing.T) {
	tests := []struct {
		name    string
		payload shared.CheckRunsPayload
		stubs   func(*httpmock.Registry)
		wantOut string
	}{
		{
			name: "no commits",
			stubs: func(reg *httpmock.Registry) {
				reg.StubResponse(200, bytes.NewBufferString(`
					{ "data": { "repository": {
						"pullRequest": { "number": 123 }
					} } }
				`))
			},
		},
		{
			name: "no checks",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{},
			},
		},
		{
			name: "some failing",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{
					{
						Name:        "sad tests",
						Status:      "completed",
						Conclusion:  "failure",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "cool tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "slow tests",
						Status:      "in_progress",
						Conclusion:  "",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
				},
			},
			wantOut: "Some checks were not successful\n1 failing, 1 successful, and 1 pending checks\n\nX  sad tests   24h0m0s  sweet link\n✓  cool tests  24h0m0s  sweet link\n.  slow tests  24h0m0s  sweet link\n",
		},
		{
			name: "some pending",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{
					{
						Name:        "rad tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "cool tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "slow tests",
						Status:      "in_progress",
						Conclusion:  "",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
				},
			},
			wantOut: "Some checks are still pending\n0 failing, 2 successful, and 1 pending checks\n\n✓  rad tests   24h0m0s  sweet link\n✓  cool tests  24h0m0s  sweet link\n.  slow tests  24h0m0s  sweet link\n",
		},
		{
			name: "all passing",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{
					{
						Name:        "rad tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "cool tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "awesome tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
				},
			},
			wantOut: "All checks were successful\n0 failing, 3 successful, and 0 pending checks\n\n✓  rad tests      24h0m0s  sweet link\n✓  cool tests     24h0m0s  sweet link\n✓  awesome tests  24h0m0s  sweet link\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, _ := iostreams.Test()
			io.SetStdoutTTY(true)

			opts := &ChecksOptions{
				IO: io,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				SelectorArg: "123",
			}

			reg := &httpmock.Registry{}
			if tt.stubs != nil {
				tt.stubs(reg)
			} else {
				reg.StubResponse(200, bytes.NewBufferString(`
				{ "data": { "repository": {
					"pullRequest": { "number": 123, "commits": { "nodes": [{"commit": {"oid": "abc"}}]} }
				} } }
			`))
			}
			reg.Register(httpmock.REST("GET", "repos/OWNER/REPO/commits/abc/check-runs"),
				httpmock.JSONResponse(tt.payload))

			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			err := checksRun(opts)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}

func Test_checksRun_nontty(t *testing.T) {
	tests := []struct {
		name    string
		payload shared.CheckRunsPayload
		stubs   func(*httpmock.Registry)
		wantOut string
	}{
		{
			name: "no commits",
			stubs: func(reg *httpmock.Registry) {
				reg.StubResponse(200, bytes.NewBufferString(`
					{ "data": { "repository": {
						"pullRequest": { "number": 123 }
					} } }
				`))
			},
		},
		{
			name: "no checks",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{},
			},
		},
		{
			name: "some failing",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{
					{
						Name:        "sad tests",
						Status:      "completed",
						Conclusion:  "failure",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "cool tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "slow tests",
						Status:      "in_progress",
						Conclusion:  "",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
				},
			},
			wantOut: "sad tests\tfail\t24h0m0s\tsweet link\ncool tests\tpass\t24h0m0s\tsweet link\nslow tests\tpending\t24h0m0s\tsweet link\n",
		},
		{
			name: "some pending",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{
					{
						Name:        "rad tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "cool tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "slow tests",
						Status:      "in_progress",
						Conclusion:  "",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
				},
			},
			wantOut: "rad tests\tpass\t24h0m0s\tsweet link\ncool tests\tpass\t24h0m0s\tsweet link\nslow tests\tpending\t24h0m0s\tsweet link\n",
		},
		{
			name: "all passing",
			payload: shared.CheckRunsPayload{
				CheckRuns: []shared.CheckRunPayload{
					{
						Name:        "rad tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "cool tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
					{
						Name:        "awesome tests",
						Status:      "completed",
						Conclusion:  "success",
						StartedAt:   startedAt(),
						CompletedAt: completedAt(),
						HtmlUrl:     "sweet link",
					},
				},
			},
			wantOut: "rad tests\tpass\t24h0m0s\tsweet link\ncool tests\tpass\t24h0m0s\tsweet link\nawesome tests\tpass\t24h0m0s\tsweet link\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, _ := iostreams.Test()
			io.SetStdoutTTY(false)

			opts := &ChecksOptions{
				IO: io,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				SelectorArg: "123",
			}

			reg := &httpmock.Registry{}
			if tt.stubs != nil {
				tt.stubs(reg)
			} else {
				reg.StubResponse(200, bytes.NewBufferString(`
				{ "data": { "repository": {
					"pullRequest": { "number": 123, "commits": { "nodes": [{"commit": {"oid": "abc"}}]} }
				} } }
			`))
			}
			reg.Register(httpmock.REST("GET", "repos/OWNER/REPO/commits/abc/check-runs"),
				httpmock.JSONResponse(tt.payload))

			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			err := checksRun(opts)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantOut, stdout.String())

		})
	}
}
