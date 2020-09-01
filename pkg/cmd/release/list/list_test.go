package list

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdList(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    ListOptions
		wantErr string
	}{
		{
			name:  "no arguments",
			args:  "",
			isTTY: true,
			want: ListOptions{
				LimitResults: 30,
			},
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

			var opts *ListOptions
			cmd := NewCmdList(f, func(o *ListOptions) error {
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

			assert.Equal(t, tt.want.LimitResults, opts.LimitResults)
		})
	}
}

func Test_listRun(t *testing.T) {
	frozenTime, err := time.Parse(time.RFC3339, "2020-08-31T15:44:24+02:00")
	require.NoError(t, err)

	tests := []struct {
		name       string
		isTTY      bool
		opts       ListOptions
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:  "list releases",
			isTTY: true,
			opts: ListOptions{
				LimitResults: 30,
			},
			wantStdout: heredoc.Doc(`
				v1.1.0                 Draft        (v1.1.0)        about 1 day ago
				The big 1.0            Latest       (v1.0.0)        about 1 day ago
				1.0 release candidate  Pre-release  (v1.0.0-pre.2)  about 1 day ago
				New features                        (v0.9.2)        about 1 day ago
			`),
			wantStderr: ``,
		},
		{
			name:  "machine-readable",
			isTTY: false,
			opts: ListOptions{
				LimitResults: 30,
			},
			wantStdout: heredoc.Doc(`
				v1.1.0	Draft	v1.1.0	2020-08-31T15:44:24+02:00
				The big 1.0	Latest	v1.0.0	2020-08-31T15:44:24+02:00
				1.0 release candidate	Pre-release	v1.0.0-pre.2	2020-08-31T15:44:24+02:00
				New features		v0.9.2	2020-08-31T15:44:24+02:00
			`),
			wantStderr: ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			createdAt := frozenTime
			if tt.isTTY {
				createdAt = time.Now().Add(time.Duration(-24) * time.Hour)
			}

			fakeHTTP := &httpmock.Registry{}
			fakeHTTP.Register(httpmock.GraphQL(`\bRepositoryReleaseList\(`), httpmock.StringResponse(fmt.Sprintf(`
			{ "data": { "repository": { "releases": {
				"nodes": [
					{
						"name": "",
						"tagName": "v1.1.0",
						"isDraft": true,
						"isPrerelease": false,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					},
					{
						"name": "The big 1.0",
						"tagName": "v1.0.0",
						"isDraft": false,
						"isPrerelease": false,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					},
					{
						"name": "1.0 release candidate",
						"tagName": "v1.0.0-pre.2",
						"isDraft": false,
						"isPrerelease": true,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					},
					{
						"name": "New features",
						"tagName": "v0.9.2",
						"isDraft": false,
						"isPrerelease": false,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					}
				]
			} } } }`, createdAt.Format(time.RFC3339))))

			tt.opts.IO = io
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := listRun(&tt.opts)
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
