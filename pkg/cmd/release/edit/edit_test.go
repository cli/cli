package edit

import (
	"bytes"
	"fmt"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"net/http"
	"testing"
)

func Test_NewCmdCreate(t *testing.T) {
	tempDir := t.TempDir()
	tf, err := ioutil.TempFile(tempDir, "release-create")
	require.NoError(t, err)
	fmt.Fprint(tf, "MY NOTES")
	tf.Close()

	tests := []struct {
		name    string
		args    string
		isTTY   bool
		stdin   string
		want    EditOptions
		wantErr string
	}{
		{
			name:    "no arguments notty",
			args:    "",
			isTTY:   false,
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:  "provide title and notes",
			args:  "12345 --title 'Some Title' --notes 'Some Notes'",
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Target:     "",
				Name:       "Some Title",
				Body:       stringPtr("Some Notes"),
				Draft:      nil,
				Prerelease: nil,
			},
		},
		{
			name:  "provide tag and target commitish",
			args:  "12345 --tag v9.8.7 --target 97ea5e77b4d61d5d80ed08f7512847dee3ec9af5",
			isTTY: false,
			want: EditOptions{
				TagName:    "v9.8.7",
				Target:     "97ea5e77b4d61d5d80ed08f7512847dee3ec9af5",
				Name:       "",
				Body:       nil,
				Draft:      nil,
				Prerelease: nil,
			},
		},
		{
			name:  "provide prerelease",
			args:  "12345 --prerelease",
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Target:     "",
				Name:       "",
				Body:       nil,
				Draft:      nil,
				Prerelease: boolPtr(true),
			},
		},
		{
			name:  "provide prerelease=false",
			args:  "12345 --prerelease=false",
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Target:     "",
				Name:       "",
				Body:       nil,
				Draft:      nil,
				Prerelease: boolPtr(false),
			},
		},
		{
			name:  "provide draft",
			args:  "12345 --draft",
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Target:     "",
				Name:       "",
				Body:       nil,
				Draft:      boolPtr(true),
				Prerelease: nil,
			},
		},
		{
			name:  "provide draft=false",
			args:  "12345 --draft=false",
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Target:     "",
				Name:       "",
				Body:       nil,
				Draft:      boolPtr(false),
				Prerelease: nil,
			},
		},
		{
			name:  "provide notes from file",
			args:  fmt.Sprintf(`12345 -F '%s'`, tf.Name()),
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Target:     "",
				Name:       "",
				Body:       stringPtr("MY NOTES"),
				Draft:      nil,
				Prerelease: nil,
			},
		},
		{
			name:  "provide notes from stdin",
			args:  "12345 -F -",
			isTTY: false,
			stdin: "MY NOTES",
			want: EditOptions{
				TagName:    "",
				Target:     "",
				Name:       "",
				Body:       stringPtr("MY NOTES"),
				Draft:      nil,
				Prerelease: nil,
			},
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

			var opts *EditOptions
			cmd := NewCmdEdit(f, func(o *EditOptions) error {
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
			assert.Equal(t, tt.want.Draft, opts.Draft)
			assert.Equal(t, tt.want.Prerelease, opts.Prerelease)
		})
	}
}

func Test_updateRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		opts       EditOptions
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:  "edit the tag name",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
				TagName:   "v1.2.3",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.3",
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the target",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
				Target:    "c0ff33",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"target_commitish": "c0ff33",
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the release name",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
				Name:      "Hot Release #1",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"name": "Hot Release #1",
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the release notes",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
				Body:      stringPtr("Release Notes:\n- Fix Bug #1\n- Fix Bug #2"),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"body": "Release Notes:\n- Fix Bug #1\n- Fix Bug #2",
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the release notes (empty)",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
				Body:      stringPtr(""),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"body": "",
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit draft (true)",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
				Draft:     boolPtr(true),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"draft": true,
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit draft (false)",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
				Draft:     boolPtr(false),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"draft": false,
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit prerelease (true)",
			isTTY: true,
			opts: EditOptions{
				ReleaseId:  "12345",
				Prerelease: boolPtr(true),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"prerelease": true,
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit prerelease (false)",
			isTTY: true,
			opts: EditOptions{
				ReleaseId:  "12345",
				Prerelease: boolPtr(false),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"prerelease": false,
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "without any changes",
			isTTY: true,
			opts: EditOptions{
				ReleaseId: "12345",
			},
			wantErr: "nothing to edit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			fakeHTTP := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, fakeHTTP)
			}
			defer fakeHTTP.Verify(t)

			tt.opts.IO = io
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := editRun(&tt.opts)
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

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
