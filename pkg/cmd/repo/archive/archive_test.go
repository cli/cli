package archive

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

// probably redundant
func TestNewCmdArchive(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		tty     bool
		output  ArchiveOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:  "valid repo",
			input: "cli/cli",
			tty:   true,
			output: ArchiveOptions{
				RepoArg: "cli/cli",
			},
		},
		{
			name:    "no argument",
			input:   "",
			wantErr: true,
			tty:     true,
			output: ArchiveOptions{
				RepoArg: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: io,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *ArchiveOptions
			cmd := NewCmdArchive(f, func(opts *ArchiveOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.RepoArg, gotOpts.RepoArg)
		})
	}
}

func Test_ArchiveRun(t *testing.T) {
	tests := []struct {
		name      string
		opts      ArchiveOptions
		httpStubs func(*httpmock.Registry)
		stdoutTTY bool
		wantOut   string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "unarchived repo tty",
			opts:      ArchiveOptions{RepoArg: "OWNER/REPO"},
			wantOut:   "✓ Archived repository OWNER/REPO\n",
			stdoutTTY: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{ "data": { "repository": {
												"id": "THE-ID",
												"isArchived": false} } }`))
				reg.Register(
					httpmock.GraphQL(`mutation ArchiveRepository\b`),
					httpmock.StringResponse(`{}`))
			},
		},
		{
			name:      "unarchived repo notty",
			opts:      ArchiveOptions{RepoArg: "OWNER/REPO"},
			stdoutTTY: false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{ "data": { "repository": {
												"id": "THE-ID",
												"isArchived": false} } }`))
				reg.Register(
					httpmock.GraphQL(`mutation ArchiveRepository\b`),
					httpmock.StringResponse(`{}`))
			},
		},
		{
			name:      "archived repo tty",
			opts:      ArchiveOptions{RepoArg: "OWNER/REPO"},
			wantOut:   "! Repository OWNER/REPO is already archived",
			stdoutTTY: true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{ "data": { "repository": {
												"id": "THE-ID",
												"isArchived": true } } }`))
			},
		},
		{
			name:      "archived repo notty",
			opts:      ArchiveOptions{RepoArg: "OWNER/REPO"},
			wantOut:   "! Repository OWNER/REPO is already archived",
			stdoutTTY: false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{ "data": { "repository": {
												"id": "THE-ID",
												"isArchived": true } } }`))
			},
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			io.SetStdoutTTY(tt.stdoutTTY)

			err := archiveRun(&tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}
