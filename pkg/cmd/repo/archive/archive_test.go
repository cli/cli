package archive

import (
	"bytes"
	"testing"

	//"github.com/cli/cli/v2/pkg/httpmock"
	//"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
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
