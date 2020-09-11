package create

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

const (
	fixtureFile = "../fixture.txt"
)

func Test_processFiles(t *testing.T) {
	fakeStdin := strings.NewReader("hey cool how is it going")
	files, err := processFiles(ioutil.NopCloser(fakeStdin), "", []string{"-"})
	if err != nil {
		t.Fatalf("unexpected error processing files: %s", err)
	}

	assert.Equal(t, 1, len(files))
	assert.Equal(t, "hey cool how is it going", files["gistfile0.txt"])
}

func Test_guessGistName_stdin(t *testing.T) {
	files := map[string]string{"gistfile0.txt": "sample content"}

	gistName := guessGistName(files)
	assert.Equal(t, "", gistName)
}

func Test_guessGistName_userFiles(t *testing.T) {
	files := map[string]string{
		"fig.txt":       "I am a fig.",
		"apple.txt":     "I am an apple.",
		"gistfile0.txt": "sample content",
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
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
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
	tests := []struct {
		name       string
		opts       *CreateOptions
		stdin      string
		wantOut    string
		wantStderr string
		wantParams map[string]interface{}
		wantErr    bool
	}{
		{
			name: "public",
			opts: &CreateOptions{
				Public:    true,
				Filenames: []string{fixtureFile},
			},
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist fixture.txt\n✓ Created gist fixture.txt\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"public": true,
				"files": map[string]interface{}{
					"fixture.txt": map[string]interface{}{
						"content": "{}",
					},
				},
			},
		},
		{
			name: "with description",
			opts: &CreateOptions{
				Description: "an incredibly interesting gist",
				Filenames:   []string{fixtureFile},
			},
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist fixture.txt\n✓ Created gist fixture.txt\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"description": "an incredibly interesting gist",
				"files": map[string]interface{}{
					"fixture.txt": map[string]interface{}{
						"content": "{}",
					},
				},
			},
		},
		{
			name: "multiple files",
			opts: &CreateOptions{
				Filenames: []string{fixtureFile, "-"},
			},
			stdin:      "cool stdin content",
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist with multiple files\n✓ Created gist fixture.txt\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"files": map[string]interface{}{
					"fixture.txt": map[string]interface{}{
						"content": "{}",
					},
					"gistfile1.txt": map[string]interface{}{
						"content": "cool stdin content",
					},
				},
			},
		},
		{
			name: "stdin arg",
			opts: &CreateOptions{
				Filenames: []string{"-"},
			},
			stdin:      "cool stdin content",
			wantOut:    "https://gist.github.com/aa5a315d61ae9438b18d\n",
			wantStderr: "- Creating gist...\n✓ Created gist\n",
			wantErr:    false,
			wantParams: map[string]interface{}{
				"files": map[string]interface{}{
					"gistfile0.txt": map[string]interface{}{
						"content": "cool stdin content",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		reg.Register(httpmock.REST("POST", "gists"),
			httpmock.JSONResponse(struct {
				Html_url string
			}{"https://gist.github.com/aa5a315d61ae9438b18d"}))

		mockClient := func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.opts.HttpClient = mockClient

		io, stdin, stdout, stderr := iostreams.Test()
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			stdin.WriteString(tt.stdin)

			if err := createRun(tt.opts); (err != nil) != tt.wantErr {
				t.Errorf("createRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			bodyBytes, _ := ioutil.ReadAll(reg.Requests[0].Body)
			reqBody := make(map[string]interface{})
			err := json.Unmarshal(bodyBytes, &reqBody)
			if err != nil {
				t.Fatalf("error decoding JSON: %v", err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantParams, reqBody)
			reg.Verify(t)
		})
	}
}

func Test_CreateRun_reauth(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(httpmock.REST("POST", "gists"), func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Request:    req,
			Header: map[string][]string{
				"X-Oauth-Scopes": {"coolScope"},
			},
			Body: ioutil.NopCloser(bytes.NewBufferString("oh no")),
		}, nil
	})

	mockClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}

	io, _, _, _ := iostreams.Test()

	opts := &CreateOptions{
		IO:         io,
		HttpClient: mockClient,
		Filenames:  []string{fixtureFile},
	}

	err := createRun(opts)
	if err == nil {
		t.Fatalf("expected oauth error")
	}

	if !strings.Contains(err.Error(), "Please re-authenticate") {
		t.Errorf("got unexpected error: %s", err)
	}
}
