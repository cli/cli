package rename

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			tty:     false,
		},
		{
			name:  "one argument",
			input: "REPO",
			output: RenameOptions{
				newRepoSelector: "REPO",
			},
		},
		{
			name:  "full flag argument",
			input: "--repo OWNER/REPO NEW_REPO",
			output: RenameOptions{
				newRepoSelector: "NEW_REPO",
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
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.newRepoSelector, gotOpts.newRepoSelector)
		})
	}
}
