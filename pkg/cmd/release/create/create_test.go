package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdCreate(t *testing.T) {
	tempDir := t.TempDir()
	tf, err := ioutil.TempFile(tempDir, "release-create")
	require.NoError(t, err)
	fmt.Fprint(tf, "MY NOTES")
	tf.Close()
	af1, err := os.Create(filepath.Join(tempDir, "windows.zip"))
	require.NoError(t, err)
	af1.Close()
	af2, err := os.Create(filepath.Join(tempDir, "linux.tgz"))
	require.NoError(t, err)
	af2.Close()

	tests := []struct {
		name    string
		args    string
		isTTY   bool
		stdin   string
		want    CreateOptions
		wantErr string
	}{
		{
			name:  "only tag name",
			args:  "v1.2.3",
			isTTY: true,
			want: CreateOptions{
				TagName:      "v1.2.3",
				Target:       "",
				Name:         "",
				Body:         "",
				BodyProvided: false,
				Draft:        false,
				Prerelease:   false,
				RepoOverride: "",
				Concurrency:  5,
				Assets:       []*shared.AssetForUpload(nil),
			},
		},
		{
			name:  "asset files",
			args:  fmt.Sprintf("v1.2.3 '%s' '%s#Linux build'", af1.Name(), af2.Name()),
			isTTY: true,
			want: CreateOptions{
				TagName:      "v1.2.3",
				Target:       "",
				Name:         "",
				Body:         "",
				BodyProvided: false,
				Draft:        false,
				Prerelease:   false,
				RepoOverride: "",
				Concurrency:  5,
				Assets: []*shared.AssetForUpload{
					{
						Name:  "windows.zip",
						Label: "",
					},
					{
						Name:  "linux.tgz",
						Label: "Linux build",
					},
				},
			},
		},
		{
			name:  "provide title and body",
			args:  "v1.2.3 -t mytitle -n mynotes",
			isTTY: true,
			want: CreateOptions{
				TagName:      "v1.2.3",
				Target:       "",
				Name:         "mytitle",
				Body:         "mynotes",
				BodyProvided: true,
				Draft:        false,
				Prerelease:   false,
				RepoOverride: "",
				Concurrency:  5,
				Assets:       []*shared.AssetForUpload(nil),
			},
		},
		{
			name:  "notes from file",
			args:  fmt.Sprintf(`v1.2.3 -F '%s'`, tf.Name()),
			isTTY: true,
			want: CreateOptions{
				TagName:      "v1.2.3",
				Target:       "",
				Name:         "",
				Body:         "MY NOTES",
				BodyProvided: true,
				Draft:        false,
				Prerelease:   false,
				RepoOverride: "",
				Concurrency:  5,
				Assets:       []*shared.AssetForUpload(nil),
			},
		},
		{
			name:  "notes from stdin",
			args:  "v1.2.3 -F -",
			isTTY: true,
			stdin: "MY NOTES",
			want: CreateOptions{
				TagName:      "v1.2.3",
				Target:       "",
				Name:         "",
				Body:         "MY NOTES",
				BodyProvided: true,
				Draft:        false,
				Prerelease:   false,
				RepoOverride: "",
				Concurrency:  5,
				Assets:       []*shared.AssetForUpload(nil),
			},
		},
		{
			name:  "set draft and prerelease",
			args:  "v1.2.3 -d -p",
			isTTY: true,
			want: CreateOptions{
				TagName:      "v1.2.3",
				Target:       "",
				Name:         "",
				Body:         "",
				BodyProvided: false,
				Draft:        true,
				Prerelease:   true,
				RepoOverride: "",
				Concurrency:  5,
				Assets:       []*shared.AssetForUpload(nil),
			},
		},
		{
			name:    "no arguments",
			args:    "",
			isTTY:   true,
			wantErr: "could not create: no tag name provided",
		},
		{
			name:  "discussion category",
			args:  "v1.2.3 --discussion-category 'General'",
			isTTY: true,
			want: CreateOptions{
				TagName:            "v1.2.3",
				Target:             "",
				Name:               "",
				Body:               "",
				BodyProvided:       false,
				Draft:              false,
				Prerelease:         false,
				RepoOverride:       "",
				Concurrency:        5,
				Assets:             []*shared.AssetForUpload(nil),
				DiscussionCategory: "General",
			},
		},
		{
			name:    "discussion category for draft release",
			args:    "v1.2.3 -d --discussion-category 'General'",
			isTTY:   true,
			wantErr: "Discussions for draft releases not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, _, _ := iostreams.Test()
			if tt.stdin == "" {
				io.SetStdinTTY(tt.isTTY)
			} else {
				io.SetStdinTTY(false)
				fmt.Fprint(stdin, tt.stdin)
			}
			io.SetStdoutTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
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
			assert.Equal(t, tt.want.Target, opts.Target)
			assert.Equal(t, tt.want.Name, opts.Name)
			assert.Equal(t, tt.want.Body, opts.Body)
			assert.Equal(t, tt.want.BodyProvided, opts.BodyProvided)
			assert.Equal(t, tt.want.Draft, opts.Draft)
			assert.Equal(t, tt.want.Prerelease, opts.Prerelease)
			assert.Equal(t, tt.want.Concurrency, opts.Concurrency)
			assert.Equal(t, tt.want.RepoOverride, opts.RepoOverride)
			assert.Equal(t, tt.want.DiscussionCategory, opts.DiscussionCategory)

			require.Equal(t, len(tt.want.Assets), len(opts.Assets))
			for i := range tt.want.Assets {
				assert.Equal(t, tt.want.Assets[i].Name, opts.Assets[i].Name)
				assert.Equal(t, tt.want.Assets[i].Label, opts.Assets[i].Label)
			}
		})
	}
}

func Test_createRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		opts       CreateOptions
		wantParams interface{}
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:  "create a release",
			isTTY: true,
			opts: CreateOptions{
				TagName:      "v1.2.3",
				Name:         "The Big 1.2",
				Body:         "* Fixed bugs",
				BodyProvided: true,
				Target:       "",
			},
			wantParams: map[string]interface{}{
				"tag_name":   "v1.2.3",
				"name":       "The Big 1.2",
				"body":       "* Fixed bugs",
				"draft":      false,
				"prerelease": false,
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: ``,
		},
		{
			name:  "with discussion category",
			isTTY: true,
			opts: CreateOptions{
				TagName:            "v1.2.3",
				Name:               "The Big 1.2",
				Body:               "* Fixed bugs",
				BodyProvided:       true,
				Target:             "",
				DiscussionCategory: "General",
			},
			wantParams: map[string]interface{}{
				"tag_name":                 "v1.2.3",
				"name":                     "The Big 1.2",
				"body":                     "* Fixed bugs",
				"draft":                    false,
				"prerelease":               false,
				"discussion_category_name": "General",
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: ``,
		},
		{
			name:  "with target commitish",
			isTTY: true,
			opts: CreateOptions{
				TagName:      "v1.2.3",
				Name:         "",
				Body:         "",
				BodyProvided: true,
				Target:       "main",
			},
			wantParams: map[string]interface{}{
				"tag_name":         "v1.2.3",
				"name":             "",
				"body":             "",
				"draft":            false,
				"prerelease":       false,
				"target_commitish": "main",
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: ``,
		},
		{
			name:  "as draft",
			isTTY: true,
			opts: CreateOptions{
				TagName:      "v1.2.3",
				Name:         "",
				Body:         "",
				BodyProvided: true,
				Draft:        true,
				Target:       "",
			},
			wantParams: map[string]interface{}{
				"tag_name":   "v1.2.3",
				"name":       "",
				"body":       "",
				"draft":      true,
				"prerelease": false,
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: ``,
		},
		{
			name:  "discussion category for draft release",
			isTTY: true,
			opts: CreateOptions{
				TagName:            "v1.2.3",
				Name:               "",
				Body:               "",
				BodyProvided:       true,
				Draft:              true,
				Target:             "",
				DiscussionCategory: "general",
			},
			wantParams: map[string]interface{}{
				"tag_name":                 "v1.2.3",
				"name":                     "",
				"body":                     "",
				"draft":                    true,
				"prerelease":               false,
				"discussion_category_name": "general",
			},
			wantErr:    "X Discussions not supported with draft releases",
			wantStdout: "",
		},
		{
			name:  "publish after uploading files",
			isTTY: true,
			opts: CreateOptions{
				TagName:      "v1.2.3",
				Name:         "",
				Body:         "",
				BodyProvided: true,
				Draft:        false,
				Target:       "",
				Assets: []*shared.AssetForUpload{
					{
						Name: "ball.tgz",
						Open: func() (io.ReadCloser, error) {
							return ioutil.NopCloser(bytes.NewBufferString(`TARBALL`)), nil
						},
					},
				},
				Concurrency: 1,
			},
			wantParams: map[string]interface{}{
				"tag_name":   "v1.2.3",
				"name":       "",
				"body":       "",
				"draft":      true,
				"prerelease": false,
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3-final\n",
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
			fakeHTTP.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.StatusStringResponse(201, `{
				"url": "https://api.github.com/releases/123",
				"upload_url": "https://api.github.com/assets/upload",
				"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
			}`))
			fakeHTTP.Register(httpmock.REST("POST", "assets/upload"), httpmock.StatusStringResponse(201, `{}`))
			fakeHTTP.Register(httpmock.REST("PATCH", "releases/123"), httpmock.StatusStringResponse(201, `{
				"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3-final"
			}`))

			tt.opts.IO = io
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := createRun(&tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			bb, err := ioutil.ReadAll(fakeHTTP.Requests[0].Body)
			require.NoError(t, err)
			var params interface{}
			err = json.Unmarshal(bb, &params)
			require.NoError(t, err)
			assert.Equal(t, tt.wantParams, params)

			if len(tt.opts.Assets) > 0 {
				q := fakeHTTP.Requests[1].URL.Query()
				assert.Equal(t, tt.opts.Assets[0].Name, q.Get("name"))
				assert.Equal(t, tt.opts.Assets[0].Label, q.Get("label"))

				bb, err := ioutil.ReadAll(fakeHTTP.Requests[2].Body)
				require.NoError(t, err)
				var updateParams interface{}
				err = json.Unmarshal(bb, &updateParams)
				require.NoError(t, err)
				assert.Equal(t, map[string]interface{}{"draft": false}, updateParams)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
