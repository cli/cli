package cmdutil

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireWriteConfirmation(t *testing.T) {
	tests := []struct {
		name        string
		envVal      string
		configValue string // set confirm_write_commands to this (empty = use default)
		canPrompt   bool
		promptRet   bool   // what Prompter.Confirm returns (only when canPrompt)
		wantErr     bool
		wantCancel  bool
	}{
		{
			name:    "env disabled skips confirmation",
			envVal:  "disabled",
			canPrompt: true,
			wantErr: false,
		},
		{
			name:    "config disabled (default) skips confirmation",
			envVal:  "",
			configValue: "disabled",
			canPrompt: true,
			wantErr: false,
		},
		{
			name:    "config enabled and can prompt, user confirms",
			envVal:  "",
			configValue: "enabled",
			canPrompt: true,
			promptRet: true,
			wantErr: false,
		},
		{
			name:    "config enabled and can prompt, user cancels",
			envVal:  "",
			configValue: "enabled",
			canPrompt: true,
			promptRet: false,
			wantErr: true,
			wantCancel: true,
		},
		{
			name:    "config enabled and cannot prompt returns error",
			envVal:  "",
			configValue: "enabled",
			canPrompt: false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal != "" {
				t.Setenv("GH_CONFIRM_WRITE_COMMANDS", tt.envVal)
			} else {
				t.Setenv("GH_CONFIRM_WRITE_COMMANDS", "")
			}

			cfg, _ := config.NewIsolatedTestConfig(t)
			if tt.configValue != "" {
				cfg.Set("", ConfirmWriteCommandsKey, tt.configValue)
			}

			ios := iostreams.Test()
			ios.SetStdinTTY(tt.canPrompt)
			ios.SetStdoutTTY(tt.canPrompt)

			pm := &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, defaultValue bool) (bool, error) {
					return tt.promptRet, nil
				},
			}

			f := &Factory{
				Config:    func() (gh.Config, error) { return cfg, nil },
				IOStreams: ios,
				Prompter:  pm,
			}

			err := RequireWriteConfirmation(f, "github.com", "create a pull request")
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantCancel {
					assert.True(t, errors.Is(err, CancelError))
				} else {
					assert.Contains(t, err.Error(), "confirm_write_commands")
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
