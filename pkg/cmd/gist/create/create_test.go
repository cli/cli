package create

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func Test_processFiles(t *testing.T) {
	fakeStdin := strings.NewReader("hey cool how is it going")
	files, err := processFiles(io.NopCloser(fakeStdin), "", []string{"-"})
	if err != nil {
		t.Fatalf("unexpected error processing files: %s", err)
	}

	assert.Equal(t, 1, len(files))
	assert.Equal(t, "hey cool how is it going", files["gistfile0.txt"].Content)
}

func Test_guessGistName_stdin(t *testing.T) {
	files := map[string]*shared.GistFile{
		"gistfile0.txt": {Content: "sample content"},
	}

	gistName := guessGistName(files)
	assert.Equal(t, "", gistName)
}

func Test_guessGistName_userFiles(t *testing.T) {
	files := map[string]*shared.GistFile{
		"fig.txt":       {Content: "I am a fig"},
		"apple.txt":     {Content: "I am an apple"},
		"gistfile0.txt": {Content: "sample content"},
	}

	gistName := guessGistName(files)
	assert.Equal(t, "apple.txt", gistName)
}

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		factory  func(*cmdutil.Factory) *cmdutil.Factory
		wants    CreateOptions
		wantsErr bool
	}{
		{
			name: "no arguments",
			cli:  "",
			wants: CreateOptions{
				Description: "",
				Public:      false,
				Filenames:   []string{""},
			},
			wantsErr: false,
		},
		{
			name: "no arguments with TTY stdin",
			factory: func(f *cmdutil.Factory) *cmdutil.Factory {
				f.IOStreams.SetStdinTTY(true)
				return f
			},
			cli: "",
			wants: CreateOptions{
				Description: "",
				Public:      false,
				Filenames:   []string{""},
			},
			wantsErr: true,
		},
		{
			name: "stdin argument",
			cli:  "-",
			wants: CreateOptions{
				Description: "",
				Public:      false,
				Filenames:   []string{"-"},
			},
			wantsErr: false,
		},
		{
			name: "with description",
			cli:  `-d "my new gist" -`,
			wants: CreateOptions{
				Description: "my new gist",
				Public:      false,
				Filenames:   []string{"-"},
			},
			wantsErr: false,
		},
		{
			name: "public",
			cli:  `--public -`,
			wants: CreateOptions{
				Description: "",
				Public:      true,
				Filenames:   []string{"-"},
			},
			wantsErr: false,
		},
		{
			name: "list of files",
			cli:  "file1.txt file2.txt",
			wants: CreateOptions{
				Description: "",
				Public:      false,
				Filenames:   []string{"file1.txt", "file2.txt"},
			},
			wantsErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			if tt.factory != nil {
				f = tt.factory(f)
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *CreateOptions
			cmd := NewCmdCreate(f, func(opts *CreateOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Description, gotOpts.Description)
			assert.Equal(t, tt.wants.Public, gotOpts.Public)
		})
	}
}

func Test_createRun(t *testing.T) {
	tempDir := t.TempDir()
	fixtureFile := filepath.Join(tempDir, "fixture.txt")
	assert.NoError(t, os.WriteFile(fixtureFile, []byte("{}"), 0644))
	emptyFile := filepath.Join(tempDir, "empty.txt")
	assert.NoError(t, os.WriteFile(emptyFile, []byte(" \t\n"), 0644))

	tests := []struct {
		name           string
		opts           *CreateOptions
		stdin          string
		wantOut        string
		wantStderr     string
		wantParams     map[string]interface{}
		wantErr        bool
		wantBrowse     string
		responseStatus int
	}{
		{
			name: "public",
			opts: &CreateOptions{
				Public:    true,
				Filenames: []string{fixtureFile},
			},
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist fixture.txt\n✓ Created public gist fixture.txt\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"description": "",
				"updated_at":  "0001-01-01T00:00:00Z",
				"public":      true,
				"files": map[string]interface{}{
					"fixture.txt": map[string]interface{}{
						"content": "{}",
					},
				},
			},
			responseStatus: http.StatusOK,
		},
		{
			name: "with description",
			opts: &CreateOptions{
				Description: "an incredibly interesting gist",
				Filenames:   []string{fixtureFile},
			},
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist fixture.txt\n✓ Created secret gist fixture.txt\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"description": "an incredibly interesting gist",
				"updated_at":  "0001-01-01T00:00:00Z",
				"public":      false,
				"files": map[string]interface{}{
					"fixture.txt": map[string]interface{}{
						"content": "{}",
					},
				},
			},
			responseStatus: http.StatusOK,
		},
		{
			name: "multiple files",
			opts: &CreateOptions{
				Filenames: []string{fixtureFile, "-"},
			},
			stdin:      "cool stdin content",
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist with multiple files\n✓ Created secret gist fixture.txt\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"description": "",
				"updated_at":  "0001-01-01T00:00:00Z",
				"public":      false,
				"files": map[string]interface{}{
					"fixture.txt": map[string]interface{}{
						"content": "{}",
					},
					"gistfile1.txt": map[string]interface{}{
						"content": "cool stdin content",
					},
				},
			},
			responseStatus: http.StatusOK,
		},
		{
			name: "file with empty content",
			opts: &CreateOptions{
				Filenames: []string{emptyFile},
			},
			wantOut: "",
			wantStderr: heredoc.Doc(`
				- Creating gist empty.txt
				X Failed to create gist: a gist file cannot be blank
			`),
			wantErr: true,
			wantParams: map[string]interface{}{
				"description": "",
				"updated_at":  "0001-01-01T00:00:00Z",
				"public":      false,
				"files": map[string]interface{}{
					"empty.txt": map[string]interface{}{"content": " \t\n"},
				},
			},
			responseStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "stdin arg",
			opts: &CreateOptions{
				Filenames: []string{"-"},
			},
			stdin:      "cool stdin content",
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist...\n✓ Created secret gist\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"description": "",
				"updated_at":  "0001-01-01T00:00:00Z",
				"public":      false,
				"files": map[string]interface{}{
					"gistfile0.txt": map[string]interface{}{
						"content": "cool stdin content",
					},
				},
			},
			responseStatus: http.StatusOK,
		},
		{
			name: "web arg",
			opts: &CreateOptions{
				WebMode:   true,
				Filenames: []string{fixtureFile},
			},
			wantOut:    "Opening https://gist.github.com/aa5a315d61ae9438b18d in your browser.\n",
			wantStderr: "- Creating gist fixture.txt\n✓ Created secret gist fixture.txt\n",
			wantErr:    false,
			wantBrowse: "https://gist.github.com/aa5a315d61ae9438b18d",
			wantParams: map[string]interface{}{
				"description": "",
				"updated_at":  "0001-01-01T00:00:00Z",
				"public":      false,
				"files": map[string]interface{}{
					"fixture.txt": map[string]interface{}{
						"content": "{}",
					},
				},
			},
			responseStatus: http.StatusOK,
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.responseStatus == http.StatusOK {
			reg.Register(
				httpmock.REST("POST", "gists"),
				httpmock.StringResponse(`{
					"html_url": "https://gist.github.com/aa5a315d61ae9438b18d"
				}`))
		} else {
			reg.Register(
				httpmock.REST("POST", "gists"),
				httpmock.StatusStringResponse(tt.responseStatus, "{}"))
		}

		mockClient := func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.opts.HttpClient = mockClient

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}

		ios, stdin, stdout, stderr := iostreams.Test()
		tt.opts.IO = ios

		browser := &browser.Stub{}
		tt.opts.Browser = browser

		_, teardown := run.Stub()
		defer teardown(t)

		t.Run(tt.name, func(t *testing.T) {
			stdin.WriteString(tt.stdin)

			if err := createRun(tt.opts); (err != nil) != tt.wantErr {
				t.Errorf("createRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			bodyBytes, _ := io.ReadAll(reg.Requests[0].Body)
			reqBody := make(map[string]interface{})
			err := json.Unmarshal(bodyBytes, &reqBody)
			if err != nil {
				t.Fatalf("error decoding JSON: %v", err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantParams, reqBody)
			reg.Verify(t)
			browser.Verify(t, tt.wantBrowse)
		})
	}
}

func Test_detectEmptyFiles(t *testing.T) {
	tests := []struct {
		content     string
		isEmptyFile bool
	}{
		{
			content:     "{}",
			isEmptyFile: false,
		},
		{
			content:     "\n\t",
			isEmptyFile: true,
		},
	}

	for _, tt := range tests {
		files := map[string]*shared.GistFile{}
		files["file"] = &shared.GistFile{
			Content: tt.content,
		}

		isEmptyFile := detectEmptyFiles(files)
		assert.Equal(t, tt.isEmptyFile, isEmptyFile)
	}
}
