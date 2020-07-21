package create

import (
	"bytes"
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

// func TestGistCreate(t *testing.T) {
// 	initBlankContext("", "OWNER/REPO", "trunk")

// 	http := initFakeHTTP()
// 	http.Register(httpmock.REST("POST", "gists"), httpmock.StringResponse(`
// 	{
// 		"html_url": "https://gist.github.com/aa5a315d61ae9438b18d"
// 	}
// 	`))

// 	output, err := RunCommand(`gist create "../test/fixtures/gistCreate.json" -d "Gist description" --public`)
// 	assert.NoError(t, err)

// 	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
// 	reqBody := make(map[string]interface{})
// 	err = json.Unmarshal(bodyBytes, &reqBody)
// 	if err != nil {
// 		t.Fatalf("error decoding JSON: %v", err)
// 	}

// 	expectParams := map[string]interface{}{
// 		"description": "Gist description",
// 		"files": map[string]interface{}{
// 			"gistCreate.json": map[string]interface{}{
// 				"content": "{}",
// 			},
// 		},
// 		"public": true,
// 	}

// 	assert.Equal(t, expectParams, reqBody)
// 	assert.Equal(t, "https://gist.github.com/aa5a315d61ae9438b18d\n", output.String())
// }

func Test_processFiles(t *testing.T) {
	fakeStdin := strings.NewReader("hey cool how is it going")
	files, err := processFiles(ioutil.NopCloser(fakeStdin), []string{"-"})
	if err != nil {
		t.Fatalf("unexpected error processing files: %s", err)
	}

	assert.Equal(t, 1, len(files))
	assert.Equal(t, "hey cool how is it going", files["gistfile0.txt"])
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

func testIO() *iostreams.IOStreams {
	tio, _, _, _ := iostreams.Test()
	return tio
}

func Test_createRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *CreateOptions
		want    func(t *testing.T)
		wantErr bool
	}{
		// TODO stdin passed as -
		// TODO stdin as |
		// TODO multiple files
		// TODO description
		// TODO public
		{
			name: "basic",
			opts: &CreateOptions{
				IO:        testIO(),
				Filenames: []string{"-"},
				HttpClient: func() (*http.Client, error) {
					reg := &httpmock.Registry{}
					reg.Register(httpmock.REST("POST", "gists"), httpmock.StringResponse(`
 	          {
 	          	"html_url": "https://gist.github.com/aa5a315d61ae9438b18d"
 	          }`))

					return &http.Client{Transport: reg}, nil
				},
			},
			want: func(t *testing.T) {

			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := createRun(tt.opts); (err != nil) != tt.wantErr {
				t.Errorf("createRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			tt.want(t)
		})
	}
}
