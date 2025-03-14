package delete

import (
	"bytes"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/cache/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    DeleteOptions
		wantsErr string
	}{
		{
			name:     "no arguments",
			cli:      "",
			wantsErr: "must provide either cache id, cache key, or use --all",
		},
		{
			name:  "id argument",
			cli:   "123",
			wants: DeleteOptions{Identifier: "123"},
		},
		{
			name:  "key argument",
			cli:   "A-Cache-Key",
			wants: DeleteOptions{Identifier: "A-Cache-Key"},
		},
		{
			name:  "delete all flag",
			cli:   "--all",
			wants: DeleteOptions{DeleteAll: true},
		},
		{
			name:  "delete all and succeed-on-no-caches flags",
			cli:   "--all --succeed-on-no-caches",
			wants: DeleteOptions{DeleteAll: true, SucceedOnNoCaches: true},
		},
		{
			name:     "succeed-on-no-caches flag",
			cli:      "--succeed-on-no-caches",
			wantsErr: "--succeed-on-no-caches must be used in conjunction with --all",
		},
		{
			name:     "succeed-on-no-caches flag and id argument",
			cli:      "--succeed-on-no-caches 123",
			wantsErr: "--succeed-on-no-caches must be used in conjunction with --all",
		},
		{
			name:     "id argument and delete all flag",
			cli:      "1 --all",
			wantsErr: "specify only one of cache id, cache key, or --all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}
			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)
			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr != "" {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wants.DeleteAll, gotOpts.DeleteAll)
			assert.Equal(t, tt.wants.SucceedOnNoCaches, gotOpts.SucceedOnNoCaches)
			assert.Equal(t, tt.wants.Identifier, gotOpts.Identifier)
		})
	}
}

func TestDeleteRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       DeleteOptions
		stubs      func(*httpmock.Registry)
		tty        bool
		wantErr    bool
		wantErrMsg string
		wantStderr string
		wantStdout string
	}{
		{
			name: "deletes cache tty",
			opts: DeleteOptions{Identifier: "123"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/actions/caches/123"),
					httpmock.StatusStringResponse(204, ""),
				)
			},
			tty:        true,
			wantStdout: "✓ Deleted 1 cache from OWNER/REPO\n",
		},
		{
			name: "deletes cache notty",
			opts: DeleteOptions{Identifier: "123"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/actions/caches/123"),
					httpmock.StatusStringResponse(204, ""),
				)
			},
			tty:        false,
			wantStdout: "",
		},
		{
			name: "non-existent cache",
			opts: DeleteOptions{Identifier: "123"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/actions/caches/123"),
					httpmock.StatusStringResponse(404, ""),
				)
			},
			wantErr:    true,
			wantErrMsg: "X Could not find a cache matching 123 in OWNER/REPO",
		},
		{
			name: "deletes all caches",
			opts: DeleteOptions{DeleteAll: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.JSONResponse(shared.CachePayload{
						ActionsCaches: []shared.Cache{
							{
								Id:             123,
								Key:            "foo",
								CreatedAt:      time.Date(2021, 1, 1, 1, 1, 1, 1, time.UTC),
								LastAccessedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
							},
							{
								Id:             456,
								Key:            "bar",
								CreatedAt:      time.Date(2021, 1, 1, 1, 1, 1, 1, time.UTC),
								LastAccessedAt: time.Date(2022, 1, 1, 1, 1, 1, 1, time.UTC),
							},
						},
						TotalCount: 2,
					}),
				)
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/actions/caches/123"),
					httpmock.StatusStringResponse(204, ""),
				)
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/actions/caches/456"),
					httpmock.StatusStringResponse(204, ""),
				)
			},
			tty:        true,
			wantStdout: "✓ Deleted 2 caches from OWNER/REPO\n",
		},
		{
			name: "attempts to delete all caches but api errors",
			opts: DeleteOptions{DeleteAll: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.StatusStringResponse(500, ""),
				)
			},
			tty:        true,
			wantErr:    true,
			wantErrMsg: "HTTP 500 (https://api.github.com/repos/OWNER/REPO/actions/caches?per_page=100)",
		},
		{
			name: "displays delete error",
			opts: DeleteOptions{Identifier: "123"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/actions/caches/123"),
					httpmock.StatusStringResponse(500, ""),
				)
			},
			wantErr:    true,
			wantErrMsg: "X Failed to delete cache: HTTP 500 (https://api.github.com/repos/OWNER/REPO/actions/caches/123)",
		},
		{
			name: "keys must be percent-encoded before being used as query params",
			opts: DeleteOptions{Identifier: "a weird＿cache+key"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.QueryMatcher("DELETE", "repos/OWNER/REPO/actions/caches", url.Values{
						"key": []string{"a weird＿cache+key"},
					}),
					httpmock.StatusStringResponse(204, ""),
				)
			},
			tty:        true,
			wantStdout: "✓ Deleted 1 cache from OWNER/REPO\n",
		},
		{
			name: "no caches to delete when deleting all",
			opts: DeleteOptions{DeleteAll: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.JSONResponse(shared.CachePayload{
						ActionsCaches: []shared.Cache{},
						TotalCount:    0,
					}),
				)
			},
			tty:        false,
			wantErr:    true,
			wantErrMsg: "X No caches to delete",
		},
		{
			name: "no caches to delete when deleting all but succeed on no cache tty",
			opts: DeleteOptions{DeleteAll: true, SucceedOnNoCaches: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.JSONResponse(shared.CachePayload{
						ActionsCaches: []shared.Cache{},
						TotalCount:    0,
					}),
				)
			},
			tty:        true,
			wantErr:    false,
			wantStdout: "✓ No caches to delete\n",
		},
		{
			name: "no caches to delete when deleting all but succeed on no cache non-tty",
			opts: DeleteOptions{DeleteAll: true, SucceedOnNoCaches: true},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/caches"),
					httpmock.JSONResponse(shared.CachePayload{
						ActionsCaches: []shared.Cache{},
						TotalCount:    0,
					}),
				)
			},
			tty:        false,
			wantErr:    false,
			wantStdout: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.stubs != nil {
				tt.stubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			tt.opts.IO = ios
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			defer reg.Verify(t)

			err := deleteRun(&tt.opts)
			if tt.wantErr {
				if tt.wantErrMsg != "" {
					assert.EqualError(t, err, tt.wantErrMsg)
				} else {
					assert.Error(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
