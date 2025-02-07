package delete

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name       string
		cli        string
		tty        bool
		want       DeleteOptions
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "valid selector",
			cli:  "123",
			tty:  true,
			want: DeleteOptions{
				Selector: "123",
			},
		},
		{
			name: "valid selector, no ID supplied",
			cli:  "",
			tty:  true,
			want: DeleteOptions{
				Selector: "",
			},
		},
		{
			name: "no ID supplied with --yes",
			cli:  "--yes",
			tty:  true,
			want: DeleteOptions{
				Selector: "",
			},
		},
		{
			name: "selector with --yes, no tty",
			cli:  "123 --yes",
			tty:  false,
			want: DeleteOptions{
				Selector: "123",
			},
		},
		{
			name: "ID arg without --yes, no tty",
			cli:  "123",
			tty:  false,
			want: DeleteOptions{
				Selector: "",
			},
			wantErr:    true,
			wantErrMsg: "--yes required when not running interactively",
		},
		{
			name: "no ID supplied with --yes, no tty",
			cli:  "--yes",
			tty:  false,
			want: DeleteOptions{
				Selector: "",
			},
			wantErr:    true,
			wantErrMsg: "id or url argument required in non-interactive mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)

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
			if tt.wantErr {
				assert.EqualError(t, err, tt.wantErrMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want.Selector, gotOpts.Selector)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	tests := []struct {
		name            string
		opts            *DeleteOptions
		cancel          bool
		httpStubs       func(*httpmock.Registry)
		mockPromptGists bool
		noGists         bool
		wantErr         bool
		wantStdout      string
		wantStderr      string
	}{
		{
			name: "successfully delete",
			opts: &DeleteOptions{
				Selector: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "gists/1234"),
					httpmock.JSONResponse(shared.Gist{ID: "1234", Files: map[string]*shared.GistFile{"cool.txt": {Filename: "cool.txt"}}}))
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(200, "{}"))
			},
			wantStdout: "✓ Gist \"cool.txt\" deleted\n",
		},
		{
			name: "successfully delete with prompt",
			opts: &DeleteOptions{
				Selector: "",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(200, "{}"))
			},
			mockPromptGists: true,
			wantStdout:      "✓ Gist \"cool.txt\" deleted\n",
		},
		{
			name: "successfully delete with --yes",
			opts: &DeleteOptions{
				Selector:  "1234",
				Confirmed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "gists/1234"),
					httpmock.JSONResponse(shared.Gist{ID: "1234", Files: map[string]*shared.GistFile{"cool.txt": {Filename: "cool.txt"}}}))
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(200, "{}"))
			},
			wantStdout: "✓ Gist \"cool.txt\" deleted\n",
		},
		{
			name: "successfully delete with prompt and --yes",
			opts: &DeleteOptions{
				Selector:  "",
				Confirmed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(200, "{}"))
			},
			mockPromptGists: true,
			wantStdout:      "✓ Gist \"cool.txt\" deleted\n",
		},
		{
			name: "cancel delete with id",
			opts: &DeleteOptions{
				Selector: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "gists/1234"),
					httpmock.JSONResponse(shared.Gist{ID: "1234", Files: map[string]*shared.GistFile{"cool.txt": {Filename: "cool.txt"}}}))
			},
			cancel:  true,
			wantErr: true,
		},
		{
			name: "cancel delete with url",
			opts: &DeleteOptions{
				Selector: "https://gist.github.com/myrepo/1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "gists/1234"),
					httpmock.JSONResponse(shared.Gist{ID: "1234", Files: map[string]*shared.GistFile{"cool.txt": {Filename: "cool.txt"}}}))
			},
			cancel:  true,
			wantErr: true,
		},
		{
			name: "cancel delete with prompt",
			opts: &DeleteOptions{
				Selector: "",
			},
			httpStubs:       func(reg *httpmock.Registry) {},
			mockPromptGists: true,
			cancel:          true,
			wantErr:         true,
		},
		{
			name: "not owned by you",
			opts: &DeleteOptions{
				Selector: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "gists/1234"),
					httpmock.JSONResponse(shared.Gist{ID: "1234", Files: map[string]*shared.GistFile{"cool.txt": {Filename: "cool.txt"}}}))
				reg.Register(httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(404, "{}"))
			},
			wantErr:    true,
			wantStderr: "unable to delete gist \"cool.txt\": either the gist is not found or it is not owned by you",
		},
		{
			name: "not found",
			opts: &DeleteOptions{
				Selector: "1234",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "gists/1234"),
					httpmock.StatusStringResponse(404, "{}"))
			},
			wantErr:    true,
			wantStderr: "not found",
		},
		{
			name: "no gists",
			opts: &DeleteOptions{
				Selector: "",
			},
			httpStubs:       func(reg *httpmock.Registry) {},
			mockPromptGists: true,
			noGists:         true,
			wantStdout:      "No gists found.\n",
		},
	}

	for _, tt := range tests {
		pm := prompter.NewMockPrompter(t)
		if !tt.opts.Confirmed {
			pm.RegisterConfirm("Delete \"cool.txt\" gist?", func(_ string, _ bool) (bool, error) {
				return !tt.cancel, nil
			})
		}

		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		if tt.mockPromptGists {
			if tt.noGists {
				reg.Register(
					httpmock.GraphQL(`query GistList\b`),
					httpmock.StringResponse(
						`{ "data": { "viewer": { "gists": { "nodes": [] }} } }`),
				)
			} else {
				sixHours, _ := time.ParseDuration("6h")
				sixHoursAgo := time.Now().Add(-sixHours)
				reg.Register(
					httpmock.GraphQL(`query GistList\b`),
					httpmock.StringResponse(fmt.Sprintf(
						`{ "data": { "viewer": { "gists": { "nodes": [
								{
									"name": "1234",
									"files": [{ "name": "cool.txt" }],
									"updatedAt": "%s",
									"isPublic": true
								}
							] } } } }`,
						sixHoursAgo.Format(time.RFC3339),
					)),
				)
				pm.RegisterSelect("Select a gist", []string{"cool.txt  about 6 hours ago"}, func(_, _ string, _ []string) (int, error) {
					return 0, nil
				})
			}

		}

		tt.opts.Prompter = pm

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdoutTTY(true)
		ios.SetStdinTTY(false)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			err := deleteRun(tt.opts)
			reg.Verify(t)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantStderr != "" {
					assert.EqualError(t, err, tt.wantStderr)
				}
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func Test_gistDelete(t *testing.T) {
	tests := []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		hostname  string
		gistID    string
		wantErr   error
	}{
		{
			name: "successful delete",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(204, "{}"),
				)
			},
			hostname: "github.com",
			gistID:   "1234",
			wantErr:  nil,
		},
		{
			name: "when an gist is not found, it returns a NotFoundError",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusStringResponse(404, "{}"),
				)
			},
			hostname: "github.com",
			gistID:   "1234",
			wantErr:  shared.NotFoundErr,
		},
		{
			name: "when there is a non-404 error deleting the gist, that error is returned",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "gists/1234"),
					httpmock.StatusJSONResponse(500, `{"message": "arbitrary error"}`),
				)
			},
			hostname: "github.com",
			gistID:   "1234",
			wantErr: api.HTTPError{
				HTTPError: &ghAPI.HTTPError{
					StatusCode: 500,
					Message:    "arbitrary error",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			tt.httpStubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			err := deleteGist(client, tt.hostname, tt.gistID)
			if tt.wantErr != nil {
				assert.ErrorAs(t, err, &tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

		})
	}
}
