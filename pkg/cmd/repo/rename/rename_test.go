package rename

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRename(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		output  RenameOptions
		errMsg  string
		tty     bool
		wantErr bool
	}{
		{
			name:    "no arguments no tty",
			input:   "",
			errMsg:  "could not prompt: proceed with prompt",
			wantErr: true,
		},
		{
			name:  "one argument",
			input: "REPO",
			output: RenameOptions{
				newRepoSelector: "REPO",
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdinTTY(tt.tty)
			io.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *RenameOptions
			cmd := NewCmdRename(f, func(opts *RenameOptions) error {
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
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.newRepoSelector, gotOpts.newRepoSelector)
		})
	}
}

func TestRenameRun(t *testing.T) {
	testCases := []struct {
		name      string
		opts      RenameOptions
		httpStubs func(*httpmock.Registry)
		askStubs  func(*prompt.AskStubber)
		wantOut   string
		tty       bool
		prompt    bool
	}{
		{
			name:    "none argument",
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n✓ Updated the \"origin\" remote",
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("NEW_REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			tty: true,
		},
		{
			name: "owner repo change name prompt",
			opts: RenameOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n",
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("NEW_REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			prompt: true,
		},
		{
			name: "owner repo change name argument ",
			opts: RenameOptions{
				newRepoSelector: "REPO",
			},
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("OWNER/REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
						  "repository": {
							"id": "THE-ID",
							"name": "REPO",
							"owner": {
							  "login": "OWNER"
							}
						  }
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
	}

	for _, tt := range testCases {
		q, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(q)
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			repo, _ := ghrepo.FromFullName("OWNER/REPO")
			return repo, nil
		}

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.Remotes = func() (context.Remotes, error) {
			r, _ := ghrepo.FromFullName("OWNER/REPO")
			var remotes context.Remotes
			remotes = append(remotes, &context.Remote{
				Remote: &git.Remote{Name: "origin"},
				Repo:   r,
			})
			return remotes, nil
		}

		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdinTTY(tt.tty)
		io.SetStdoutTTY(tt.tty)
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := renameRun(&tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}
