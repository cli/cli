package create

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name      string
		tty       bool
		cli       string
		wantsErr  bool
		errMsg    string
		wantsOpts CreateOptions
	}{
		{
			name:      "no args tty",
			tty:       true,
			cli:       "",
			wantsOpts: CreateOptions{Interactive: true},
		},
		{
			name:     "no args no-tty",
			tty:      false,
			cli:      "",
			wantsErr: true,
			errMsg:   "at least one argument required in non-interactive mode",
		},
		{
			name: "new repo from remote",
			cli:  "NEWREPO --public --clone",
			wantsOpts: CreateOptions{
				Name:   "NEWREPO",
				Public: true,
				Clone:  true},
		},
		{
			name:     "no visibility",
			tty:      true,
			cli:      "NEWREPO",
			wantsErr: true,
			errMsg:   "`--public`, `--private`, or `--internal` required when not running interactively",
		},
		{
			name:     "multiple visibility",
			tty:      true,
			cli:      "NEWREPO --public --private",
			wantsErr: true,
			errMsg:   "`--public`, `--private`, or `--internal` required when not running interactively",
		},
		{
			name: "new remote from local",
			cli:  "--source=/path/to/repo --private",
			wantsOpts: CreateOptions{
				Private: true,
				Source:  "/path/to/repo"},
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

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				opts = o
				return nil
			})

			// TODO STUPID HACK
			// cobra aggressively adds help to all commands. since we're not running through the root command
			// (which manages help when running for real) and since create has a '-h' flag (for homepage),
			// cobra blows up when it tried to add a help flag and -h is already in use. This hack adds a
			// dummy help flag with a random shorthand to get around this.
			cmd.Flags().BoolP("help", "x", false, "")

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantsOpts.Interactive, opts.Interactive)
			assert.Equal(t, tt.wantsOpts.Source, opts.Source)
			assert.Equal(t, tt.wantsOpts.Name, opts.Name)
			assert.Equal(t, tt.wantsOpts.Public, opts.Public)
			assert.Equal(t, tt.wantsOpts.Internal, opts.Internal)
			assert.Equal(t, tt.wantsOpts.Private, opts.Private)
			assert.Equal(t, tt.wantsOpts.Clone, opts.Clone)
		})
	}
}
