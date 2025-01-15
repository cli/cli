package list

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
				LimitResults:       30,
				ExcludeDrafts:      false,
				ExcludePreReleases: false,
				Order:              "desc",
			},
		},
		{
			name: "exclude drafts",
			args: "--exclude-drafts",
			want: ListOptions{
				LimitResults:       30,
				ExcludeDrafts:      true,
				ExcludePreReleases: false,
				Order:              "desc",
			},
		},
		{
			name: "exclude pre-releases",
			args: "--exclude-pre-releases",
			want: ListOptions{
				LimitResults:       30,
				ExcludeDrafts:      false,
				ExcludePreReleases: true,
				Order:              "desc",
			},
		},
		{
			name: "with order",
			args: "--order asc",
			want: ListOptions{
				LimitResults:       30,
				ExcludeDrafts:      false,
				ExcludePreReleases: false,
				Order:              "asc",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.LimitResults, opts.LimitResults)
			assert.Equal(t, tt.want.ExcludeDrafts, opts.ExcludeDrafts)
			assert.Equal(t, tt.want.ExcludePreReleases, opts.ExcludePreReleases)
			assert.Equal(t, tt.want.Order, opts.Order)
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
				TITLE                  TYPE         TAG NAME      PUBLISHED
				v1.1.0                 Draft        v1.1.0        about 1 day ago
				The big 1.0            Latest       v1.0.0        about 1 day ago
				1.0 release candidate  Pre-release  v1.0.0-pre.2  about 1 day ago
				New features                        v0.9.2        about 1 day ago
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
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

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
						"isLatest": false,
						"isDraft": true,
						"isPrerelease": false,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					},
					{
						"name": "The big 1.0",
						"tagName": "v1.0.0",
						"isLatest": true,
						"isDraft": false,
						"isPrerelease": false,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					},
					{
						"name": "1.0 release candidate",
						"tagName": "v1.0.0-pre.2",
						"isLatest": false,
						"isDraft": false,
						"isPrerelease": true,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					},
					{
						"name": "New features",
						"tagName": "v0.9.2",
						"isLatest": false,
						"isDraft": false,
						"isPrerelease": false,
						"createdAt": "%[1]s",
						"publishedAt": "%[1]s"
					}
				]
			} } } }`, createdAt.Format(time.RFC3339))))

			tt.opts.IO = ios
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

func TestExportReleases(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	createdAt, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	publishedAt, _ := time.Parse(time.RFC3339, "2024-02-01T00:00:00Z")
	rs := []Release{{
		Name:         "v1",
		TagName:      "tag",
		IsDraft:      true,
		IsLatest:     false,
		IsPrerelease: true,
		CreatedAt:    createdAt,
		PublishedAt:  publishedAt,
	}}
	exporter := cmdutil.NewJSONExporter()
	exporter.SetFields(releaseFields)
	require.NoError(t, exporter.Write(ios, rs))
	require.JSONEq(t,
		`[{"createdAt":"2024-01-01T00:00:00Z","isDraft":true,"isLatest":false,"isPrerelease":true,"name":"v1","publishedAt":"2024-02-01T00:00:00Z","tagName":"tag"}]`,
		stdout.String())
}
