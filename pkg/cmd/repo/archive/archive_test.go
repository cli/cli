package archive

import (
	"bytes"
	"net/http"
	"testing"
    "fmt"

	//"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

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
			name: "valid repo",
			input:  "cli/cli",
			tty:  true,
			output: ArchiveOptions{
				RepoArg: "cli/cli",
			},
		},
		{
			name: "no argument",
			input: "",
			tty:  true,
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

func Test_ArchiveRun(t *testing.T)  {
	tests := []struct {
		name       string
		opts       *ArchiveOptions
		repoName   string
		stdoutTTY  bool
		wantOut    string
		wantErr    bool
        isArchived bool
	}{
		{
			name: "unarchived repo",
            repoName: "OWNER/REPO",
            isArchived:     false,
            wantOut: "✓ Archived repository OWNER/REPO\n",
            stdoutTTY:     true,
		},
        {
            name: "archived repo",
            repoName:       "OWNER/REPO",
            isArchived:     true,
            wantOut: "! Repository OWNER/REPO is already archived\n",
            stdoutTTY:     true,
        },
	}

	for _, tt := range tests {
		if tt.opts == nil {
            tt.opts = &ArchiveOptions{}
		}

		if tt.repoName == "" {
			tt.repoName = "OWNER/REPO"
		}
        tt.opts.RepoArg = "OWNER/REPO"

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			repo, _ := ghrepo.FromFullName(tt.repoName)
			return repo, nil
		}

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`query RepositoryInfo\b`),
			httpmock.StringResponse(
                fmt.Sprintf(`
                    { "data": {
                        "repository": {
                            "id": "THE-ID",
                            "isArchived": %t
                    } } }`, tt.isArchived)))

        if !tt.isArchived {
            reg.Register(
                httpmock.GraphQL(`mutation ArchiveRepository\b`),
                httpmock.StringResponse(`{}`))
        }

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, _, stderr := iostreams.Test()
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			if err := archiveRun(tt.opts); (err != nil) != tt.wantErr {
				t.Errorf("archiveRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.wantOut, stderr.String())
            reg.Verify(t)
		})
	}
}
