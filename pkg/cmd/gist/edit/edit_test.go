package edit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_getFilesToAdd(t *testing.T) {
	filename := "gist-test.txt"

	gf, err := getFilesToAdd(filename, []byte("hello"))
	require.NoError(t, err)

	assert.Equal(t, map[string]*gistFileToUpdate{
		filename: {
			NewFilename: filename,
			Content:     "hello",
		},
	}, gf)
}

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    EditOptions
		wantsErr bool
	}{
		{
			name: "no flags",
			cli:  "123",
			wants: EditOptions{
				Selector: "123",
			},
		},
		{
			name: "filename",
			cli:  "123 --filename cool.md",
			wants: EditOptions{
				Selector:     "123",
				EditFilename: "cool.md",
			},
		},
		{
			name: "add",
			cli:  "123 --add cool.md",
			wants: EditOptions{
				Selector:    "123",
				AddFilename: "cool.md",
			},
		},
		{
			name: "add with source",
			cli:  "123 --add cool.md -",
			wants: EditOptions{
				Selector:    "123",
				AddFilename: "cool.md",
				SourceFile:  "-",
			},
		},
		{
			name: "description",
			cli:  `123 --desc "my new description"`,
			wants: EditOptions{
				Selector:    "123",
				Description: "my new description",
			},
		},
		{
			name: "remove",
			cli:  "123 --remove cool.md",
			wants: EditOptions{
				Selector:       "123",
				RemoveFilename: "cool.md",
			},
		},
		{
			name:     "add and remove are mutually exclusive",
			cli:      "123 --add cool.md --remove great.md",
			wantsErr: true,
		},
		{
			name:     "filename and remove are mutually exclusive",
			cli:      "123 --filename cool.md --remove great.md",
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *EditOptions
			cmd := NewCmdEdit(f, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			require.Equal(t, tt.wants.EditFilename, gotOpts.EditFilename)
			require.Equal(t, tt.wants.AddFilename, gotOpts.AddFilename)
			require.Equal(t, tt.wants.Selector, gotOpts.Selector)
			require.Equal(t, tt.wants.RemoveFilename, gotOpts.RemoveFilename)
		})
	}
}

func Test_editRun(t *testing.T) {
	fileToAdd := filepath.Join(t.TempDir(), "gist-test.txt")
	err := os.WriteFile(fileToAdd, []byte("hello"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name          string
		opts          *EditOptions
		gist          *shared.Gist
		httpStubs     func(*httpmock.Registry)
		prompterStubs func(*prompter.MockPrompter)
		nontty        bool
		stdin         string
		wantErr       string
		wantParams    map[string]interface{}
	}{
		{
			name:    "no such gist",
			wantErr: "gist not found: 1234",
		},
		{
			name: "one file",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Filename: "cicada.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			wantParams: map[string]interface{}{
				"description": "",
				"files": map[string]interface{}{
					"cicada.txt": map[string]interface{}{
						"content":  "new file content",
						"filename": "cicada.txt",
					},
				},
			},
		},
		{
			name: "multiple files, submit",
			prompterStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Edit which file?",
					[]string{"cicada.txt", "unix.md"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "unix.md")
					})
				pm.RegisterSelect("What next?",
					editNextOptions,
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "Submit")
					})
			},
			gist: &shared.Gist{
				ID:          "1234",
				Description: "catbug",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Filename: "cicada.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
					},
					"unix.md": {
						Filename: "unix.md",
						Content:  "meow",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			wantParams: map[string]interface{}{
				"description": "catbug",
				"files": map[string]interface{}{
					"cicada.txt": map[string]interface{}{
						"content":  "bwhiizzzbwhuiiizzzz",
						"filename": "cicada.txt",
					},
					"unix.md": map[string]interface{}{
						"content":  "new file content",
						"filename": "unix.md",
					},
				},
			},
		},
		{
			name: "multiple files, cancel",
			prompterStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect("Edit which file?",
					[]string{"cicada.txt", "unix.md"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "unix.md")
					})
				pm.RegisterSelect("What next?",
					editNextOptions,
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "Cancel")
					})
			},
			wantErr: "CancelError",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Filename: "cicada.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
					"unix.md": {
						Filename: "unix.md",
						Content:  "meow",
						Type:     "application/markdown",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
		},
		{
			name: "not change",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Filename: "cicada.txt",
						Content:  "new file content",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
		},
		{
			name: "another user's gist",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Filename: "cicada.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat2"},
			},
			wantErr: "you do not own this gist",
		},
		{
			name: "add file to existing gist",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			opts: &EditOptions{
				AddFilename: fileToAdd,
			},
		},
		{
			name: "change description",
			opts: &EditOptions{
				Description: "my new description",
			},
			gist: &shared.Gist{
				ID:          "1234",
				Description: "my old description",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			wantParams: map[string]interface{}{
				"description": "my new description",
				"files": map[string]interface{}{
					"sample.txt": map[string]interface{}{
						"content":  "new file content",
						"filename": "sample.txt",
					},
				},
			},
		},
		{
			name: "add file to existing gist from source parameter",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			opts: &EditOptions{
				AddFilename: "from_source.txt",
				SourceFile:  fileToAdd,
			},
			wantParams: map[string]interface{}{
				"description": "",
				"files": map[string]interface{}{
					"from_source.txt": map[string]interface{}{
						"content":  "hello",
						"filename": "from_source.txt",
					},
				},
			},
		},
		{
			name: "add file to existing gist from stdin",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			opts: &EditOptions{
				AddFilename: "from_source.txt",
				SourceFile:  "-",
			},
			stdin: "data from stdin",
			wantParams: map[string]interface{}{
				"description": "",
				"files": map[string]interface{}{
					"from_source.txt": map[string]interface{}{
						"content":  "data from stdin",
						"filename": "from_source.txt",
					},
				},
			},
		},
		{
			name: "remove file, file does not exist",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			opts: &EditOptions{
				RemoveFilename: "sample2.txt",
			},
			wantErr: "gist has no file \"sample2.txt\"",
		},
		{
			name: "remove file from existing gist",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
					"sample2.txt": {
						Filename: "sample2.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			opts: &EditOptions{
				RemoveFilename: "sample2.txt",
			},
			wantParams: map[string]interface{}{
				"description": "",
				"files": map[string]interface{}{
					"sample.txt": map[string]interface{}{
						"filename": "sample.txt",
						"content":  "bwhiizzzbwhuiiizzzz",
					},
					"sample2.txt": nil,
				},
			},
		},
		{
			name: "edit gist using file from source parameter",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			opts: &EditOptions{
				SourceFile: fileToAdd,
			},
			wantParams: map[string]interface{}{
				"description": "",
				"files": map[string]interface{}{
					"sample.txt": map[string]interface{}{
						"content":  "hello",
						"filename": "sample.txt",
					},
				},
			},
		},
		{
			name: "edit gist using stdin",
			gist: &shared.Gist{
				ID: "1234",
				Files: map[string]*shared.GistFile{
					"sample.txt": {
						Filename: "sample.txt",
						Content:  "bwhiizzzbwhuiiizzzz",
						Type:     "text/plain",
					},
				},
				Owner: &shared.GistOwner{Login: "octocat"},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("POST", "gists/1234"),
					httpmock.StatusStringResponse(201, "{}"))
			},
			opts: &EditOptions{
				SourceFile: "-",
			},
			stdin: "data from stdin",
			wantParams: map[string]interface{}{
				"description": "",
				"files": map[string]interface{}{
					"sample.txt": map[string]interface{}{
						"content":  "data from stdin",
						"filename": "sample.txt",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.gist == nil {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.StatusStringResponse(404, "Not Found"))
		} else {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.JSONResponse(tt.gist))
			reg.Register(httpmock.GraphQL(`query UserCurrent\b`),
				httpmock.StringResponse(`{"data":{"viewer":{"login":"octocat"}}}`))
		}

		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}

		if tt.opts == nil {
			tt.opts = &EditOptions{}
		}

		tt.opts.Edit = func(_, _, _ string, _ *iostreams.IOStreams) (string, error) {
			return "new file content", nil
		}

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		ios, stdin, stdout, stderr := iostreams.Test()
		stdin.WriteString(tt.stdin)
		ios.SetStdoutTTY(!tt.nontty)
		ios.SetStdinTTY(!tt.nontty)
		tt.opts.IO = ios
		tt.opts.Selector = "1234"

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}

		t.Run(tt.name, func(t *testing.T) {
			pm := prompter.NewMockPrompter(t)
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			err := editRun(tt.opts)
			reg.Verify(t)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)

			if tt.wantParams != nil {
				bodyBytes, _ := io.ReadAll(reg.Requests[2].Body)
				reqBody := make(map[string]interface{})
				err = json.Unmarshal(bodyBytes, &reqBody)
				if err != nil {
					t.Fatalf("error decoding JSON: %v", err)
				}
				assert.Equal(t, tt.wantParams, reqBody)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())
		})
	}
}
