package create

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants createOptions
	}{
		{
			name: "no flags",
			cli:  "",
			wants: createOptions{
				Ref:         "",
				Environment: "",
			},
		},
		{
			name: "ref flag",
			cli:  "--ref main",
			wants: createOptions{
				Ref:         "main",
				Environment: "",
			},
		},
		{
			name: "environment flag",
			cli:  "--environment production",
			wants: createOptions{
				Ref:         "",
				Environment: "production",
			},
		},
		{
			name: "ref and environment flags",
			cli:  "--ref main --environment production",
			wants: createOptions{
				Ref:         "main",
				Environment: "production",
			},
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

			var gotOpts createOptions
			cmd := NewCmdCreate(f, func(opts *createOptions) error {
				gotOpts = *opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()

			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Ref, gotOpts.Ref)
			assert.Equal(t, tt.wants.Environment, gotOpts.Environment)
		})
	}
}
