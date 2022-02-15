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

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
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
			name:  "no arguments tty",
			args:  "",
			isTTY: true,
			want: CreateOptions{
				TagName:      "",
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
			name:    "no arguments notty",
			args:    "",
			isTTY:   false,
			wantErr: "tag required when not running interactively",
		},
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
			wantErr: "discussions for draft releases not supported",
		},
		{
			name:  "generate release notes",
			args:  "v1.2.3 --generate-notes",
			isTTY: true,
			want: CreateOptions{
				TagName:       "v1.2.3",
				Target:        "",
				Name:          "",
				Body:          "",
				BodyProvided:  true,
				Draft:         false,
				Prerelease:    false,
				RepoOverride:  "",
				Concurrency:   5,
				Assets:        []*shared.AssetForUpload(nil),
				GenerateNotes: true,
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
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
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
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":   "v1.2.3",
						"name":       "The Big 1.2",
						"body":       "* Fixed bugs",
						"draft":      false,
						"prerelease": false,
					}, params)
				}))
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
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":                 "v1.2.3",
						"name":                     "The Big 1.2",
						"body":                     "* Fixed bugs",
						"draft":                    false,
						"prerelease":               false,
						"discussion_category_name": "General",
					}, params)
				}))
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
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":         "v1.2.3",
						"draft":            false,
						"prerelease":       false,
						"target_commitish": "main",
					}, params)
				}))
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
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":   "v1.2.3",
						"draft":      true,
						"prerelease": false,
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: ``,
		},
		{
			name:  "with generate notes",
			isTTY: true,
			opts: CreateOptions{
				TagName:       "v1.2.3",
				Name:          "",
				Body:          "",
				Target:        "",
				BodyProvided:  true,
				GenerateNotes: true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":               "v1.2.3",
						"draft":                  false,
						"prerelease":             false,
						"generate_release_notes": true,
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantErr:    "",
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
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":   "v1.2.3",
						"draft":      true,
						"prerelease": false,
					}, params)
				}))
				reg.Register(httpmock.REST("POST", "assets/upload"), func(req *http.Request) (*http.Response, error) {
					q := req.URL.Query()
					assert.Equal(t, "ball.tgz", q.Get("name"))
					assert.Equal(t, "", q.Get("label"))
					return &http.Response{
						StatusCode: 201,
						Request:    req,
						Body:       ioutil.NopCloser(bytes.NewBufferString(`{}`)),
						Header: map[string][]string{
							"Content-Type": {"application/json"},
						},
					}, nil
				})
				reg.Register(httpmock.REST("PATCH", "releases/123"), httpmock.RESTPayload(201, `{
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3-final"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"draft": false,
					}, params)
				}))
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3-final\n",
			wantStderr: ``,
		},
		{
			name:  "upload files and create discussion",
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
				DiscussionCategory: "general",
				Concurrency:        1,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.RESTPayload(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":                 "v1.2.3",
						"draft":                    true,
						"prerelease":               false,
						"discussion_category_name": "general",
					}, params)
				}))
				reg.Register(httpmock.REST("POST", "assets/upload"), func(req *http.Request) (*http.Response, error) {
					q := req.URL.Query()
					assert.Equal(t, "ball.tgz", q.Get("name"))
					assert.Equal(t, "", q.Get("label"))
					return &http.Response{
						StatusCode: 201,
						Request:    req,
						Body:       ioutil.NopCloser(bytes.NewBufferString(`{}`)),
						Header: map[string][]string{
							"Content-Type": {"application/json"},
						},
					}, nil
				})
				reg.Register(httpmock.REST("PATCH", "releases/123"), httpmock.RESTPayload(201, `{
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3-final"
				}`, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"draft":                    false,
						"discussion_category_name": "general",
					}, params)
				}))
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

			err := createRun(&tt.opts)
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

func Test_createRun_interactive(t *testing.T) {
	tests := []struct {
		name       string
		httpStubs  func(*httpmock.Registry)
		askStubs   func(*prompt.AskStubber)
		runStubs   func(*run.CommandStubber)
		opts       *CreateOptions
		wantParams map[string]interface{}
		wantOut    string
		wantErr    string
	}{
		{
			name: "create a release from existing tag",
			opts: &CreateOptions{},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Choose a tag").
					AssertOptions([]string{"v1.2.3", "v1.2.2", "v1.0.0", "v0.1.2", "Create a new tag"}).
					AnswerWith("v1.2.3")
				as.StubPrompt("Title (optional)").AnswerWith("")
				as.StubPrompt("Release notes").
					AssertOptions([]string{"Write my own", "Write using generated notes as template", "Leave blank"}).
					AnswerWith("Leave blank")
				as.StubPrompt("Is this a prerelease?").AnswerWith(false)
				as.StubPrompt("Submit?").
					AssertOptions([]string{"Publish release", "Save as draft", "Cancel"}).AnswerWith("Publish release")
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag --list`, 1, "")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "repos/OWNER/REPO/tags"), httpmock.StatusStringResponse(200, `[
					{ "name": "v1.2.3" }, { "name": "v1.2.2" }, { "name": "v1.0.0" }, { "name": "v0.1.2" }
				]`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
					httpmock.StatusStringResponse(200, `{
						"name": "generated name",
						"body": "generated body"
					}`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.StatusStringResponse(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`))
			},
			wantOut: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
		},
		{
			name: "create a release from new tag",
			opts: &CreateOptions{},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Choose a tag").AnswerWith("Create a new tag")
				as.StubPrompt("Tag name").AnswerWith("v1.2.3")
				as.StubPrompt("Title (optional)").AnswerWith("")
				as.StubPrompt("Release notes").
					AssertOptions([]string{"Write my own", "Write using generated notes as template", "Leave blank"}).
					AnswerWith("Leave blank")
				as.StubPrompt("Is this a prerelease?").AnswerWith(false)
				as.StubPrompt("Submit?").AnswerWith("Publish release")
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag --list`, 1, "")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "repos/OWNER/REPO/tags"), httpmock.StatusStringResponse(200, `[
					{ "name": "v1.2.2" }, { "name": "v1.0.0" }, { "name": "v0.1.2" }
				]`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
					httpmock.StatusStringResponse(200, `{
						"name": "generated name",
						"body": "generated body"
					}`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.StatusStringResponse(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`))
			},
			wantOut: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
		},
		{
			name: "create a release using generated notes",
			opts: &CreateOptions{
				TagName: "v1.2.3",
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Title (optional)").AnswerDefault()
				as.StubPrompt("Release notes").
					AssertOptions([]string{"Write my own", "Write using generated notes as template", "Leave blank"}).
					AnswerWith("Write using generated notes as template")
				as.StubPrompt("Is this a prerelease?").AnswerWith(false)
				as.StubPrompt("Submit?").AnswerWith("Publish release")
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag --list`, 1, "")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
					httpmock.StatusStringResponse(200, `{
						"name": "generated name",
						"body": "generated body"
					}`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"),
					httpmock.StatusStringResponse(201, `{
						"url": "https://api.github.com/releases/123",
						"upload_url": "https://api.github.com/assets/upload",
						"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
					}`))
			},
			wantParams: map[string]interface{}{
				"body":       "generated body",
				"draft":      false,
				"name":       "generated name",
				"prerelease": false,
				"tag_name":   "v1.2.3",
			},
			wantOut: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
		},
		{
			name: "create a release using commit log as notes",
			opts: &CreateOptions{
				TagName: "v1.2.3",
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Title (optional)").AnswerDefault()
				as.StubPrompt("Release notes").
					AssertOptions([]string{"Write my own", "Write using commit log as template", "Leave blank"}).
					AnswerWith("Write using commit log as template")
				as.StubPrompt("Is this a prerelease?").AnswerWith(false)
				as.StubPrompt("Submit?").AnswerWith("Publish release")
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag --list`, 1, "")
				rs.Register(`git describe --tags --abbrev=0 HEAD\^`, 0, "v1.2.2\n")
				rs.Register(`git .+log .+v1\.2\.2\.\.HEAD$`, 0, "commit subject\n\ncommit body\n")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
					httpmock.StatusStringResponse(404, `{}`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"),
					httpmock.StatusStringResponse(201, `{
						"url": "https://api.github.com/releases/123",
						"upload_url": "https://api.github.com/assets/upload",
						"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
					}`))
			},
			wantParams: map[string]interface{}{
				"body":       "* commit subject\n\n  commit body\n  ",
				"draft":      false,
				"prerelease": false,
				"tag_name":   "v1.2.3",
			},
			wantOut: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
		},
		{
			name: "create using annotated tag as notes",
			opts: &CreateOptions{
				TagName: "v1.2.3",
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Title (optional)").AnswerDefault()
				as.StubPrompt("Release notes").
					AssertOptions([]string{"Write my own", "Write using git tag message as template", "Leave blank"}).
					AnswerWith("Write using git tag message as template")
				as.StubPrompt("Is this a prerelease?").AnswerWith(false)
				as.StubPrompt("Submit?").AnswerWith("Publish release")
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag --list`, 0, "hello from annotated tag")
				rs.Register(`git describe --tags --abbrev=0 v1\.2\.3\^`, 1, "")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL("RepositoryFindRef"),
					httpmock.StringResponse(`{"data":{"repository":{"ref": {"id": "tag id"}}}}`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
					httpmock.StatusStringResponse(404, `{}`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"),
					httpmock.StatusStringResponse(201, `{
						"url": "https://api.github.com/releases/123",
						"upload_url": "https://api.github.com/assets/upload",
						"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
					}`))
			},
			wantParams: map[string]interface{}{
				"body":       "hello from annotated tag",
				"draft":      false,
				"prerelease": false,
				"tag_name":   "v1.2.3",
			},
			wantOut: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
		},
		{
			name: "error when unpublished local tag and target not specified",
			opts: &CreateOptions{
				TagName: "v1.2.3",
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag --list`, 0, "tag exists")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL("RepositoryFindRef"),
					httpmock.StringResponse(`{"data":{"repository":{"ref": {"id": ""}}}}`))
			},
			wantErr: "tag v1.2.3 exists locally but has not been pushed to OWNER/REPO, please push it before continuing or specify the `--target` flag to create a new tag",
		},
		{
			name: "create a release when unpublished local tag and target specified",
			opts: &CreateOptions{
				TagName: "v1.2.3",
				Target:  "main",
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Title (optional)").AnswerWith("")
				as.StubPrompt("Release notes").
					AssertOptions([]string{"Write my own", "Write using generated notes as template", "Write using git tag message as template", "Leave blank"}).
					AnswerWith("Leave blank")
				as.StubPrompt("Is this a prerelease?").AnswerWith(false)
				as.StubPrompt("Submit?").AnswerWith("Publish release")
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git tag --list`, 0, "tag exists")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases/generate-notes"),
					httpmock.StatusStringResponse(200, `{
						"name": "generated name",
						"body": "generated body"
					}`))
				reg.Register(httpmock.REST("POST", "repos/OWNER/REPO/releases"), httpmock.StatusStringResponse(201, `{
					"url": "https://api.github.com/releases/123",
					"upload_url": "https://api.github.com/assets/upload",
					"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
				}`))
			},
			wantParams: map[string]interface{}{
				"draft":            false,
				"prerelease":       false,
				"tag_name":         "v1.2.3",
				"target_commitish": "main",
			},
			wantOut: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
		},
	}
	for _, tt := range tests {
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdoutTTY(true)
		ios.SetStdinTTY(true)
		ios.SetStderrTTY(true)
		tt.opts.IO = ios

		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		tt.httpStubs(reg)
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("OWNER/REPO")
		}

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.Edit = func(_, _, val string, _ io.Reader, _, _ io.Writer) (string, error) {
			return val, nil
		}

		t.Run(tt.name, func(t *testing.T) {
			as := prompt.NewAskStubber(t)
			if tt.askStubs != nil {
				tt.askStubs(as)
			}

			rs, teardown := run.Stub()
			defer teardown(t)
			if tt.runStubs != nil {
				tt.runStubs(rs)
			}

			err := createRun(tt.opts)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantParams != nil {
				var r *http.Request
				for _, req := range reg.Requests {
					if req.URL.Path == "/repos/OWNER/REPO/releases" {
						r = req
						break
					}
				}
				if r == nil {
					t.Fatalf("no http requests for creating a release found")
				}
				bb, err := ioutil.ReadAll(r.Body)
				assert.NoError(t, err)
				var params map[string]interface{}
				err = json.Unmarshal(bb, &params)
				assert.NoError(t, err)
				assert.Equal(t, tt.wantParams, params)
			}

			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, "", stderr.String())
		})
	}
}
