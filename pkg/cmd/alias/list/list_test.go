package list

import (
	"bytes"
	"io"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliasList(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		isTTY      bool
		wantErr    bool
		wantStdout string
		wantStderr string
	}{
		{
			name:       "empty",
			config:     "",
			isTTY:      true,
			wantErr:    true,
			wantStdout: "",
			wantStderr: "",
		},
		{
			name: "some",
			config: heredoc.Doc(`
				aliases:
				  co: pr checkout
				  gc: "!gh gist create \"$@\" | pbcopy"
			`),
			isTTY:      true,
			wantStdout: "co: pr checkout\ngc: '!gh gist create \"$@\" | pbcopy'\n",
			wantStderr: "",
		},
		{
			name: "multiline",
			config: heredoc.Doc(`
				aliases:
				  one: "foo\nbar\n"
				  two: |-
				    !chicken
				    coop
			`),
			isTTY:      true,
			wantStdout: "one: |\n    foo\n    bar\ntwo: |-\n    !chicken\n    coop\n",
			wantStderr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewFromString(tt.config)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			factory := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					return cfg, nil
				},
			}

			cmd := NewCmdList(factory, nil)
			cmd.SetArgs([]string{})

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err := cmd.ExecuteC()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
