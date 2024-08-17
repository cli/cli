package delete

import (
	"bytes"
	"io"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  DeleteOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "no arguments",
			input:   "",
			wantErr: true,
			errMsg:  "specify an alias to delete or `--all`",
		},
		{
			name:  "specified alias",
			input: "co",
			output: DeleteOptions{
				Name: "co",
			},
		},
		{
			name:  "all flag",
			input: "--all",
			output: DeleteOptions{
				All: true,
			},
		},
		{
			name:    "specified alias and all flag",
			input:   "co --all",
			wantErr: true,
			errMsg:  "cannot use `--all` with alias name",
		},
		{
			name:    "too many arguments",
			input:   "il co",
			wantErr: true,
			errMsg:  "accepts at most 1 arg(s), received 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.Name, gotOpts.Name)
			assert.Equal(t, tt.output.All, gotOpts.All)
		})
	}
}

func TestDeleteRun(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		isTTY       bool
		opts        *DeleteOptions
		wantAliases map[string]string
		wantStdout  string
		wantStderr  string
		wantErrMsg  string
	}{
		{
			name: "delete alias",
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
			wantStderr: "✓ Deleted alias co; was pr checkout\n",
		},
		{
			name: "delete all aliases",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			isTTY: true,
			opts: &DeleteOptions{
				All: true,
			},
			wantAliases: map[string]string{},
			wantStderr:  "✓ Deleted alias co; was pr checkout\n✓ Deleted alias il; was issue list\n",
		},
		{
			name: "delete alias that does not exist",
			config: heredoc.Doc(`
				aliases:
				  il: issue list
				  co: pr checkout
			`),
			isTTY: true,
			opts: &DeleteOptions{
				Name: "unknown",
			},
			wantAliases: map[string]string{
				"il": "issue list",
				"co": "pr checkout",
			},
			wantErrMsg: "no such alias unknown",
		},
		{
			name:  "delete all aliases when none exist",
			isTTY: true,
			opts: &DeleteOptions{
				All: true,
			},
			wantAliases: map[string]string{},
			wantErrMsg:  "no aliases configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)
			tt.opts.IO = ios

			cfg := config.NewFromString(tt.config)
			tt.opts.Config = func() (gh.Config, error) {
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
				assert.Equal(t, 1, len(writeCalls))
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantAliases, cfg.Aliases().All())
		})
	}
}
