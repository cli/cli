package rename

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdRename(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  RenameOptions
		errMsg  string
		tty     bool
		wantErr bool
	}{
		{
			name:  "no flags",
			input: "123",
			output: RenameOptions{
				Selector: "123",
			},
		},
		{
			name:  "no new filename",
			input: "123 old.md",
			output: RenameOptions{
				Selector:    "123",
				OldFileName: "old.md",
			},
		},
		{
			name:  "with both old and new filename",
			input: "123 old.md new.md",
			output: RenameOptions{
				Selector:    "123",
				OldFileName: "old.md",
				NewFileName: "new.md",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

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
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Selector, gotOpts.Selector)
			assert.Equal(t, tt.wants.OldFileName, gotOpts.OldFileName)
			assert.Equal(t, tt.wants.NewFileName, gotOpts.NewFileName)
		})
	}
}
