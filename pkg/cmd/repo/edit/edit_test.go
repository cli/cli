package edit

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts EditOptions
		wantErr  string
	}{
		{
			name:    "no argument",
			args:    "",
			wantErr: "at least one flag is required",
		},
		{
			name: "change repo description",
			args: "--description hello",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Description: sp("hello"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(true)
			io.SetStdinTTY(true)
			io.SetStderrTTY(true)

			f := &cmdutil.Factory{
				IOStreams: io,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				HttpClient: func() (*http.Client, error) {
					return nil, nil
				},
			}

			argv, err := shlex.Split(tt.args)
			assert.NoError(t, err)

			var gotOpts *EditOptions
			cmd := NewCmdEdit(f, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, ghrepo.FullName(tt.wantOpts.Repository), ghrepo.FullName(gotOpts.Repository))
			assert.Equal(t, tt.wantOpts.Edits, gotOpts.Edits)
		})
	}
}

func Test_editRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        EditOptions
		httpStubs   func(*testing.T, *httpmock.Registry)
		wantsStderr string
		wantsErr    string
	}{
		{
			name: "change name and description",
			opts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Homepage:    sp("newURL"),
					Description: sp("hello world!"),
				},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 2, len(payload))
						assert.Equal(t, "newURL", payload["homepage"])
						assert.Equal(t, "hello world!", payload["description"])
					}))
			},
		},
		{
			name: "add and remove topics",
			opts: EditOptions{
				Repository:   ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				AddTopics:    []string{"topic1", "topic2"},
				RemoveTopics: []string{"topic3"},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/topics"),
					httpmock.StringResponse(`{"names":["topic2", "topic3", "go"]}`))
				r.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/topics"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 1, len(payload))
						assert.Equal(t, []interface{}{"topic2", "go", "topic1"}, payload["names"])
					}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, httpReg)
			}

			opts := &tt.opts
			opts.HTTPClient = &http.Client{Transport: httpReg}

			err := editRun(context.Background(), opts)
			if tt.wantsErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}
		})
	}
}

func sp(v string) *string {
	return &v
}
