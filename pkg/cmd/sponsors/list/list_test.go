package list

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants string
	}{
		{
			name:  "happy path",
			cli:   "octocat",
			wants: "octocat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants, gotOpts.Username)
		})
	}
}

func Test_listRun(t *testing.T) {
	tests := []struct {
		name  string
		opts  *ListOptions
		wants []string
	}{
		{
			name: "happy path",
			opts: &ListOptions{
				Username: "octocat",
				Sponsors: []string{},
			},
			wants: []string{"GitHub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := listRun(tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wants[0], tt.opts.Sponsors[0])
		})
	}
}
