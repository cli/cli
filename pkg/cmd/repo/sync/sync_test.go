package sync

import (
	"bytes"
	"testing"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdSync(t *testing.T) {
	tests := []struct {
		name     string
		tty      bool
		input    string
		output   SyncOptions
		wantsErr bool
		errMsg   string
	}{
		{
			name:   "no argument",
			tty:    true,
			input:  "",
			output: SyncOptions{},
		},
		{
			name:  "destination repo",
			tty:   true,
			input: "cli/cli",
			output: SyncOptions{
				DestArg: "cli/cli",
			},
		},
		{
			name:  "source repo",
			tty:   true,
			input: "--source cli/cli",
			output: SyncOptions{
				SrcArg: "cli/cli",
			},
		},
		{
			name:  "branch",
			tty:   true,
			input: "--branch trunk",
			output: SyncOptions{
				Branch: "trunk",
			},
		},
		{
			name:  "force",
			tty:   true,
			input: "--force",
			output: SyncOptions{
				Force: true,
			},
		},
		{
			name:  "confirm",
			tty:   true,
			input: "--confirm",
			output: SyncOptions{
				SkipConfirm: true,
			},
		},
		{
			name:     "notty without confirm",
			tty:      false,
			input:    "",
			wantsErr: true,
			errMsg:   "`--confirm` required when not running interactively",
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
			var gotOpts *SyncOptions
			cmd := NewCmdSync(f, func(opts *SyncOptions) error {
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
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.DestArg, gotOpts.DestArg)
			assert.Equal(t, tt.output.SrcArg, gotOpts.SrcArg)
			assert.Equal(t, tt.output.Branch, gotOpts.Branch)
			assert.Equal(t, tt.output.Force, gotOpts.Force)
			assert.Equal(t, tt.output.SkipConfirm, gotOpts.SkipConfirm)
		})
	}
}

func Test_SyncRun(t *testing.T) {

}
