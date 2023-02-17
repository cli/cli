package delete

import (
	"bytes"
	"io"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAliasDelete(t *testing.T) {
	_ = config.StubWriteConfig(t)

	tests := []struct {
		name       string
		config     string
		cli        string
		isTTY      bool
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name:       "no aliases",
			config:     "",
			cli:        "co",
			isTTY:      true,
			wantStdout: "",
			wantStderr: "",
			wantErr:    "no such alias co",
		},
		{
			name: "delete one",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			cli:        "co",
			isTTY:      true,
			wantStdout: "",
			wantStderr: "✓ Deleted alias co; was pr checkout\n",
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
				Config: func() (config.Config, error) {
					return cfg, nil
				},
			}

			cmd := NewCmdDelete(factory, nil)

			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
