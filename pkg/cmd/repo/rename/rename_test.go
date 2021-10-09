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