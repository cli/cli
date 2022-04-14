package create

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  CreateOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			errMsg:  "cannot create label: name argument required",
		},
		{
			name:   "name argument",
			input:  "test",
			output: CreateOptions{Name: "test"},
		},
		{
			name:   "description flag",
			input:  "test --description 'some description'",
			output: CreateOptions{Name: "test", Description: "some description"},
		},
		{
			name:   "color flag",
			input:  "test --color FFFFFF",
			output: CreateOptions{Name: "test", Color: "FFFFFF"},
		},
		{
			name:   "color flag with pound sign",
			input:  "test --color '#AAAAAA'",
			output: CreateOptions{Name: "test", Color: "AAAAAA"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}
			argv, err := shlex.Split(tt.input)
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
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.Color, gotOpts.Color)
			assert.Equal(t, tt.output.Description, gotOpts.Description)
			assert.Equal(t, tt.output.Name, gotOpts.Name)
		})
	}
}

func TestCreateRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *CreateOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout string
	}{
		{
			name: "creates label",
			tty:  true,
			opts: &CreateOptions{Name: "test", Description: "some description"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, "{}"),
				)
			},
			wantStdout: "✓ Label \"test\" created in OWNER/REPO\n",
		},
		{
			name: "creates label notty",
			tty:  false,
			opts: &CreateOptions{Name: "test", Description: "some description"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.StatusStringResponse(201, "{}"),
				)
			},
			wantStdout: "",
		},
		{
			name: "creates existing label",
			opts: &CreateOptions{Name: "test", Description: "some description", Force: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/labels"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(422, `{"message":"Validation Failed","errors":[{"resource":"Label","code":"already_exists","field":"name"}]}`),
						"Content-Type",
						"application/json",
					),
				)
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO/labels/test"),
					httpmock.StatusStringResponse(201, "{}"),
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			io, _, stdout, _ := iostreams.Test()
			io.SetStdoutTTY(tt.tty)
			io.SetStdinTTY(tt.tty)
			io.SetStderrTTY(tt.tty)
			tt.opts.IO = io
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			defer reg.Verify(t)
			err := createRun(tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())

			bodyBytes, _ := ioutil.ReadAll(reg.Requests[0].Body)
			reqBody := map[string]string{}
			err = json.Unmarshal(bodyBytes, &reqBody)
			assert.NoError(t, err)
			assert.NotEqual(t, "", reqBody["color"])
			assert.Equal(t, "some description", reqBody["description"])
			assert.Equal(t, "test", reqBody["name"])
		})
	}
}
