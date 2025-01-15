package edit

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
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

func Test_NewCmdEdit(t *testing.T) {
	tempDir := t.TempDir()
	tf, err := os.CreateTemp(tempDir, "release-create")
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
			args:  "v1.2.3 --title 'Some Title' --notes 'Some Notes'",
			isTTY: false,
			want: EditOptions{
				TagName: "",
				Name:    stringPtr("Some Title"),
				Body:    stringPtr("Some Notes"),
			},
		},
		{
			name:  "provide discussion category",
			args:  "v1.2.3 --discussion-category some-category",
			isTTY: false,
			want: EditOptions{
				TagName:            "",
				DiscussionCategory: stringPtr("some-category"),
			},
		},
		{
			name:  "provide tag and target commitish",
			args:  "v1.2.3 --tag v9.8.7 --target 97ea5e77b4d61d5d80ed08f7512847dee3ec9af5",
			isTTY: false,
			want: EditOptions{
				TagName: "v9.8.7",
				Target:  "97ea5e77b4d61d5d80ed08f7512847dee3ec9af5",
			},
		},
		{
			name:  "provide prerelease",
			args:  "v1.2.3 --prerelease",
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Prerelease: boolPtr(true),
			},
		},
		{
			name:  "provide prerelease=false",
			args:  "v1.2.3 --prerelease=false",
			isTTY: false,
			want: EditOptions{
				TagName:    "",
				Prerelease: boolPtr(false),
			},
		},
		{
			name:  "provide draft",
			args:  "v1.2.3 --draft",
			isTTY: false,
			want: EditOptions{
				TagName: "",
				Draft:   boolPtr(true),
			},
		},
		{
			name:  "provide draft=false",
			args:  "v1.2.3 --draft=false",
			isTTY: false,
			want: EditOptions{
				TagName: "",
				Draft:   boolPtr(false),
			},
		},
		{
			name:  "latest",
			args:  "v1.2.3 --latest",
			isTTY: false,
			want: EditOptions{
				TagName:  "",
				IsLatest: boolPtr(true),
			},
		},
		{
			name:  "not latest",
			args:  "v1.2.3 --latest=false",
			isTTY: false,
			want: EditOptions{
				TagName:  "",
				IsLatest: boolPtr(false),
			},
		},
		{
			name:  "provide notes from file",
			args:  fmt.Sprintf(`v1.2.3 -F '%s'`, tf.Name()),
			isTTY: false,
			want: EditOptions{
				TagName: "",
				Body:    stringPtr("MY NOTES"),
			},
		},
		{
			name:  "provide notes from stdin",
			args:  "v1.2.3 -F -",
			isTTY: false,
			stdin: "MY NOTES",
			want: EditOptions{
				TagName: "",
				Body:    stringPtr("MY NOTES"),
			},
		},
		{
			name:  "verify-tag",
			args:  "v1.2.0 --tag=v1.1.0 --verify-tag",
			isTTY: false,
			want: EditOptions{
				TagName:   "v1.1.0",
				VerifyTag: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, _, _ := iostreams.Test()
			if tt.stdin == "" {
				ios.SetStdinTTY(tt.isTTY)
			} else {
				ios.SetStdinTTY(false)
				fmt.Fprint(stdin, tt.stdin)
			}
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

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
			assert.Equal(t, tt.want.DiscussionCategory, opts.DiscussionCategory)
			assert.Equal(t, tt.want.Draft, opts.Draft)
			assert.Equal(t, tt.want.Prerelease, opts.Prerelease)
			assert.Equal(t, tt.want.IsLatest, opts.IsLatest)
			assert.Equal(t, tt.want.VerifyTag, opts.VerifyTag)
		})
	}
}

func Test_editRun(t *testing.T) {
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
				TagName: "v1.2.4",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.4",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the target",
			isTTY: true,
			opts: EditOptions{
				Target: "c0ff33",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":         "v1.2.3",
						"target_commitish": "c0ff33",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the release name",
			isTTY: true,
			opts: EditOptions{
				Name: stringPtr("Hot Release #1"),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.3",
						"name":     "Hot Release #1",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the discussion category",
			isTTY: true,
			opts: EditOptions{
				DiscussionCategory: stringPtr("some-category"),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":                 "v1.2.3",
						"discussion_category_name": "some-category",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the latest marker",
			isTTY: false,
			opts: EditOptions{
				IsLatest: boolPtr(true),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":    "v1.2.3",
						"make_latest": "true",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the release name (empty)",
			isTTY: true,
			opts: EditOptions{
				Name: stringPtr(""),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.3",
						"name":     "",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the release notes",
			isTTY: true,
			opts: EditOptions{
				Body: stringPtr("Release Notes:\n- Fix Bug #1\n- Fix Bug #2"),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.3",
						"body":     "Release Notes:\n- Fix Bug #1\n- Fix Bug #2",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit the release notes (empty)",
			isTTY: true,
			opts: EditOptions{
				Body: stringPtr(""),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.3",
						"body":     "",
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit draft (true)",
			isTTY: true,
			opts: EditOptions{
				Draft: boolPtr(true),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.3",
						"draft":    true,
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit draft (false)",
			isTTY: true,
			opts: EditOptions{
				Draft: boolPtr(false),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name": "v1.2.3",
						"draft":    false,
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit prerelease (true)",
			isTTY: true,
			opts: EditOptions{
				Prerelease: boolPtr(true),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":   "v1.2.3",
						"prerelease": true,
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "edit prerelease (false)",
			isTTY: true,
			opts: EditOptions{
				Prerelease: boolPtr(false),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				mockSuccessfulEditResponse(reg, func(params map[string]interface{}) {
					assert.Equal(t, map[string]interface{}{
						"tag_name":   "v1.2.3",
						"prerelease": false,
					}, params)
				})
			},
			wantStdout: "https://github.com/OWNER/REPO/releases/tag/v1.2.3\n",
			wantStderr: "",
		},
		{
			name:  "error when remote tag does not exist and verify-tag flag is set",
			isTTY: true,
			opts: EditOptions{
				TagName:   "v1.2.4",
				VerifyTag: true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL("RepositoryFindRef"),
					httpmock.StringResponse(`{"data":{"repository":{"ref": {"id": ""}}}}`))
			},
			wantErr:    "tag v1.2.4 doesn't exist in the repo OWNER/REPO, aborting due to --verify-tag flag",
			wantStdout: "",
			wantStderr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			fakeHTTP := &httpmock.Registry{}
			defer fakeHTTP.Verify(t)
			shared.StubFetchRelease(t, fakeHTTP, "OWNER", "REPO", "v1.2.3", `{
				"id": 12345,
				"tag_name": "v1.2.3"
			}`)
			if tt.httpStubs != nil {
				tt.httpStubs(t, fakeHTTP)
			}

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("OWNER/REPO")
			}

			err := editRun("v1.2.3", &tt.opts)
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

func mockSuccessfulEditResponse(reg *httpmock.Registry, cb func(params map[string]interface{})) {
	matcher := httpmock.REST("PATCH", "repos/OWNER/REPO/releases/12345")
	responder := httpmock.RESTPayload(201, `{
		"html_url": "https://github.com/OWNER/REPO/releases/tag/v1.2.3"
	}`, cb)
	reg.Register(matcher, responder)
}

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}
