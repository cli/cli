package delete

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdDelete(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    DeleteOptions
		wantErr string
	}{
		{
			name:  "version argument",
			args:  "v1.2.3",
			isTTY: true,
			want: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: false,
			},
		},
		{
			name:  "skip confirm",
			args:  "v1.2.3 -y",
			isTTY: true,
			want: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: true,
			},
		},
		{
			name:    "no arguments",
			args:    "",
			isTTY:   true,
			wantErr: "accepts 1 arg(s), received 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *DeleteOptions
			cmd := NewCmdDelete(f, func(o *DeleteOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.TagName, opts.TagName)
			assert.Equal(t, tt.want.SkipConfirm, opts.SkipConfirm)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		opts       DeleteOptions
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:  "skipping confirmation",
			isTTY: true,
			opts: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: true,
			},
			wantStdout: ``,
			wantStderr: heredoc.Doc(`
				✓ Deleted release v1.2.3
				! Note that the v1.2.3 git tag still remains in the repository
			`),
		},
		{
			name:  "non-interactive",
			isTTY: false,
			opts: DeleteOptions{
				TagName:     "v1.2.3",
				SkipConfirm: false,
			},
			wantStdout: ``,
			wantStderr: ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			fakeHTTP := &httpmock.Registry{}
			fakeHTTP.Register(httpmock.REST("GET", "repos/OWNER/REPO/releases/tags/v1.2.3"), httpmock.StringResponse(`{
				"tag_name": "v1.2.3",
				"draft": false,
				"url": "https://api.github.com/repos/OWNER/REPO/releases/23456"
			}`))
			fakeHTTP.Register(httpmock.REST("DELETE", "repos/OWNER/REPO/releases/23456"), httpmock.StatusStringResponse(204, ""))

			tt.opts.IO = io
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := deleteRun(&tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
