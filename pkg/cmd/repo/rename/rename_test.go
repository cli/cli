package rename

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdRename(t *testing.T) {
	testCases := []struct {
		name     string
		args     string
		wantOpts RenameOptions
		wantErr  string
	}{
		{
			name:    "no arguments",
			args:    "",
			wantErr: "cannot rename: repository argument required",
		},
		{
			name: "correct argument",
			args: "OWNER/REPO REPOS",
			wantOpts: RenameOptions{
				oldRepoName: "OWNER/REPO",
				newRepoName: "REPOS",
			},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, stdout, stderr := iostreams.Test()
			fac := &cmdutil.Factory{IOStreams: io}

			var opts *RenameOptions
			cmd := NewCmdRename(fac, func(co *RenameOptions) error {
				opts = co
				return nil
			})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(stdin)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())

			assert.Equal(t, tt.wantOpts.oldRepoName, opts.oldRepoName)
			assert.Equal(t, tt.wantOpts.newRepoName, opts.newRepoName)
		})
	}
}

// func TestRenameRun(t *testing.T) {
// 	testCases := []struct {
// 		name      string
// 		opts      RenameOptions
// 		httpStubs func(*httpmock.Registry)
// 		stdoutTTY bool
// 		wantOut   string
// 	}{
// 		{
// 			name: "owner repo change name tty",
// 			opts: RenameOptions{
// 				oldRepoName: "OWNER/REPO",
// 				newRepoName: "NEW_REPO",
// 			},
// 			wantOut: "✓ Renamed repository OWNER/NEW_REPO",
// 			httpStubs: func(reg *httpmock.Registry) {
// 				reg.Register(
// 					httpmock.GraphQL(`query RepositoryInfo\b`),
// 					httpmock.StringResponse(`{ "data": 
// 												{ "repository": {
// 													"id": "THE-ID"} } }`))
// 				reg.Register(
// 					httpmock.REST("PATCH", "repos/OWNER/REPO"),
// 					httpmock.StatusStringResponse(204, "{}"))
// 			},
// 			stdoutTTY: true,
// 		},
// 		{
// 			name: "owner repo change name notty",
// 			opts: RenameOptions{
// 				oldRepoName: "OWNER/REPO",
// 				newRepoName: "NEW_REPO",
// 			},
// 			wantOut: "✓ Renamed repository pxrth9/team2-hack",
// 			httpStubs: func(reg *httpmock.Registry) {
// 				reg.Register(
// 					httpmock.GraphQL(`query RepositoryInfo\b`),
// 					httpmock.StringResponse(`{ "data": 
// 												{ "repository": {
// 													"id": "THE-ID"} } }`))
// 				reg.Register(
// 					httpmock.REST("PATCH", "repos/OWNER/REPO"),
// 					httpmock.StatusStringResponse(200, "{}"))
// 			},
// 			stdoutTTY: false,
// 		},
// 		{
// 			name: "nonowner repo change name tty",
// 			opts: RenameOptions{
// 				oldRepoName: "NON_OWNER/REPO",
// 				newRepoName: "NEW_REPO",
// 			},
// 			wantOut: "X you do not own this repository",
// 			httpStubs: func(reg *httpmock.Registry) {
// 				reg.Register(
// 					httpmock.GraphQL(`query RepositoryInfo\b`),
// 					httpmock.StringResponse(`{ "data": 
// 												{ "repository": {
// 													"id": "THE-ID"} } }`))
// 				reg.Register(
// 					httpmock.REST("PATCH", "repos/NON_OWNER/REPO"),
// 					httpmock.StatusStringResponse(200, "{}"))
// 			},
// 			stdoutTTY: true,
// 		},
// 		{
// 			name: "non owner repo change name notty",
// 			opts: RenameOptions{
// 				oldRepoName: "NON_OWNER/REPO",
// 				newRepoName: "NEW_REPO",
// 			},
// 			wantOut: "X you do not own this repository",
// 			httpStubs: func(reg *httpmock.Registry) {
// 				reg.Register(
// 					httpmock.GraphQL(`query RepositoryInfo\b`),
// 					httpmock.StringResponse(`{ "data": 
// 												{ "repository": {
// 													"id": "THE-ID"} } }`))
// 				reg.Register(
// 					httpmock.REST("PATCH", "repos/NON_OWNER/REPO"),
// 					httpmock.StatusStringResponse(200, "{}"))
// 			},
// 			stdoutTTY: false,
// 		},
// 	}

// 	for _, tt := range testCases {
// 		t.Run(tt.name, func(t * testing.T) {
// 			reg := &httpmock.Registry{}
// 			defer reg.Verify(t)
// 			if tt.httpStubs != nil {
// 				tt.httpStubs(reg)
// 			}

// 			io, _, stdout, _ := iostreams.Test()

// 			tt.opts.HttpClient = func() (*http.Client, error) {
// 				return &http.Client{Transport: reg}, nil
// 			}

// 			tt.opts.IO = io
// 			io.SetStderrTTY(tt.stdoutTTY)
// 			io.SetStdoutTTY(tt.stdoutTTY)
// 			err := renameRun(&tt.opts)
// 			assert.NoError(t, err)
// 			assert.Equal(t, tt.wantOut, stdout.String())
// 		})
// 	}
// }
