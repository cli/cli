package rename

// import (
// 	"fmt"
// 	"net/http"
// 	"testing"

// 	"github.com/cli/cli/v2/pkg/cmdutil"
// 	"github.com/cli/cli/v2/internal/ghrepo"
// 	"github.com/cli/cli/v2/pkg/httpmock"
// 	"github.com/cli/cli/v2/pkg/iostreams"
// 	"github.com/cli/cli/v2/pkg/prompt"

// 	"github.com/google/shlex"
// 	"github.com/stretchr/testify/assert"
// 	"github.com/stretchr/testify/require"
// )

// func TestNewCmdRename(t *testing.T) {
// 	testCases := []struct {
// 		name     string
// 		args     string
// 		wantOpts RenameOptions
// 		wantErr  string
// 	}{
// 		{
// 			name:    "no arguments",
// 			args:    "",
// 			wantErr: "cannot rename: repository argument required",
// 		},
// 		{
// 			name: "correct argument",
// 			args: "OWNER/REPO REPOS",
// 			wantOpts: RenameOptions{
// 				oldRepoSelector: "OWNER/REPO",
// 				newRepoSelector: "REPOS",
// 			},
// 		},
// 	}
// 	for _, tt := range testCases {
// 		t.Run(tt.name, func(t *testing.T) {
// 			io, stdin, stdout, stderr := iostreams.Test()
// 			fac := &cmdutil.Factory{IOStreams: io}

// 			var opts *RenameOptions
// 			cmd := NewCmdRename(fac, func(co *RenameOptions) error {
// 				opts = co
// 				return nil
// 			})

// 			argv, err := shlex.Split(tt.args)
// 			require.NoError(t, err)
// 			cmd.SetArgs(argv)

// 			cmd.SetIn(stdin)
// 			cmd.SetOut(stdout)
// 			cmd.SetErr(stderr)

// 			_, err = cmd.ExecuteC()
// 			if tt.wantErr != "" {
// 				assert.EqualError(t, err, tt.wantErr)
// 				return
// 			} else {
// 				assert.NoError(t, err)
// 			}

// 			assert.Equal(t, "", stdout.String())
// 			assert.Equal(t, "", stderr.String())

// 			assert.Equal(t, tt.wantOpts.oldRepoSelector, opts.oldRepoSelector)
// 			assert.Equal(t, tt.wantOpts.newRepoSelector, opts.newRepoSelector)
// 		})
// 	}
// }

// func TestRenameRun(t *testing.T) {
// 	testCases := []struct {
// 		name      string
// 		opts      RenameOptions
// 		httpStubs func(*httpmock.Registry)
// 		askStubs  func(*prompt.AskStubber)
// 		wantOut   string
// 		tty       bool
// 		prompt    bool
// 	}{
// 		{
// 			name: "owner repo change name using flag",
// 			opts: RenameOptions{
// 				oldRepoSelector: "OWNER/REPO",
// 				newRepoSelector: "NEW_REPO",
// 				flagRepo:        true,
// 			},
// 			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n",
// 			httpStubs: func(reg *httpmock.Registry) {
// 				reg.Register(
// 					httpmock.GraphQL(`query UserCurrent\b`),
// 					httpmock.StringResponse(`{"data":{"viewer":{"login":"OWNER"}}}`))
// 				reg.Register(
// 					httpmock.REST("PATCH", "repos/OWNER/REPO"),
// 					httpmock.StatusStringResponse(204, "{}"))
// 			},
// 			tty: true,
// 		},
// 		{
// 			name: "owner repo change name prompt",
// 			opts: RenameOptions{
// 				BaseRepo: func() (ghrepo.Interface, error) {
// 					return ghrepo.New("OWNER", "REPO"), nil
// 				},
// 				oldRepoSelector: "NEW_REPO",
// 			},
// 			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n",
// 			askStubs: func(q *prompt.AskStubber) {
// 				q.StubOne("NEW_REPO")
// 			},
// 			httpStubs: func(reg *httpmock.Registry) {
// 				reg.Register(
// 					httpmock.REST("PATCH", "repos/OWNER/REPO"),
// 					httpmock.StatusStringResponse(204, "{}"))
// 			},
// 			prompt: true,
// 		},
// 		{
// 			name: "owner repo change name argument ",
// 			opts: RenameOptions{
// 				newRepoSelector: "REPO",
// 				flagRepo: false,
// 			},
// 			askStubs: func(q *prompt.AskStubber) {
// 				q.StubOne("OWNER/REPO")
// 			},
// 			httpStubs: func(reg *httpmock.Registry) {
// 				reg.Register(
// 					httpmock.GraphQL(`query RepositoryInfo\b`),
// 					httpmock.StringResponse(`
// 					{
// 						"data": {
// 						  "repository": {
// 							"id": "THE-ID",
// 							"name": "REPO",
// 							"owner": {
// 							  "login": "OWNER"
// 							}
// 						  }
// 						}
// 					}`))
// 				reg.Register(
// 					httpmock.REST("PATCH", "repos/OWNER/REPO"),
// 					httpmock.StatusStringResponse(204, "{}"))
// 			},
// 		},
// 	}

// 	for _, tt := range testCases {
// 		q, teardown := prompt.InitAskStubber()
// 		defer teardown()
// 		if tt.askStubs != nil {
// 			tt.askStubs(q)
// 		}

// 		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
// 			repo, _ := ghrepo.FromFullName(tt.opts.oldRepoSelector)
// 			return repo, nil
// 		}

// 		reg := &httpmock.Registry{}
// 		if tt.httpStubs != nil {
// 			tt.httpStubs(reg)
// 		}
// 		tt.opts.HttpClient = func() (*http.Client, error) {
// 			return &http.Client{Transport: reg}, nil
// 		}

// 		io, _, stdout, _ := iostreams.Test()
// 		io.SetStdinTTY(tt.tty)
// 		io.SetStdoutTTY(tt.tty)
// 		tt.opts.IO = io

// 		t.Run(tt.name, func(t *testing.T) {
// 			defer reg.Verify(t)
// 			err := renameRun(&tt.opts)
// 			assert.NoError(t, err)
// 			assert.Equal(t, tt.wantOut, stdout.String())
// 		})
// 	}
// }
