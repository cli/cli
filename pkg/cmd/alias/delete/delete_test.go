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
		{
			name: "delete all aliases",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			cli:        "--all",
			isTTY:      true,
			wantStdout: "",
			wantStderr: "✓ Deleted alias il; was issue list\n✓ Deleted alias co; was pr checkout\n",
		},
		{
			name: "delete too many arguments error",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			cli:        "il co",
			isTTY:      true,
			wantStdout: "",
			wantStderr: "",
			wantErr:    "too many arguments",
		},
		{
			name: "delete --all with alias name error",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			cli:        "il --all",
			isTTY:      true,
			wantStdout: "",
			wantStderr: "",
			wantErr:    "cannot use `--all` with alias name",
		},
		{
			name: "delete without specification error",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			cli:        "",
			isTTY:      true,
			wantStdout: "",
			wantStderr: "",
			wantErr:    "specify an alias to delete or `--all`",
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

func TestDeleteRun(t *testing.T) {
	tests := []struct {
		name               string
		config             string
		isTTY              bool
		opts               *DeleteOptions
		wantWriteCallCount int
		wantAliases        map[string]string
		wantStdout         string
		wantStderr         string
		wantErrMsg         string
	}{
		{
			name: "delete one alias",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			isTTY: true,
			opts: &DeleteOptions{
				Name: "co",
				All:  false,
			},
			wantAliases: map[string]string{
				"il": "issue list",
			},
			wantWriteCallCount: 1,
			wantStdout:         "",
			wantStderr:         "✓ Deleted alias co; was pr checkout\n",
			wantErrMsg:         "",
		},
		{
			name: "delete all aliases with all flag",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			isTTY: true,
			opts: &DeleteOptions{
				All: true,
			},
			wantWriteCallCount: 2,
			wantAliases:        map[string]string{},
			wantStdout:         "",
			wantStderr:         "✓ Deleted alias il; was issue list\n✓ Deleted alias co; was pr checkout\n",
			wantErrMsg:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)
			tt.opts.IO = ios

			cfg := config.NewFromString(tt.config)
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			err := deleteRun(tt.opts)
			if tt.wantErrMsg != "" {
				assert.EqualError(t, err, tt.wantErrMsg)
				writeCalls := cfg.WriteCalls()
				assert.Equal(t, 0, len(writeCalls))
			} else {
				assert.NoError(t, err)
				writeCalls := cfg.WriteCalls()
				assert.Equal(t, tt.wantWriteCallCount, len(writeCalls))
			}

			aliasCfg := cfg.Aliases()
			aliases := aliasCfg.All()
			for name, expansion := range aliases {
				wantExpansion, ok := tt.wantAliases[name]
				if !ok {
					t.Errorf("expected not to have alias '%s', but it was registered", name)
				}
				assert.Equal(t, wantExpansion, expansion)
			}
		})
	}
}
