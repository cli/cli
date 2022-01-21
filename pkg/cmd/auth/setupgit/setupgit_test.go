package setupgit

import (
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockGitConfigurer struct {
	setupErr error
}

func (gf *mockGitConfigurer) Setup(hostname, username, authToken string) error {
	return gf.setupErr
}

func Test_setupGitRun(t *testing.T) {
	tests := []struct {
		name           string
		opts           *SetupGitOptions
		expectedErr    string
		expectedErrOut string
	}{
		{
			name: "opts.Config returns an error",
			opts: &SetupGitOptions{
				Config: func() (config.Config, error) {
					return nil, fmt.Errorf("oops")
				},
			},
			expectedErr: "oops",
		},
		{
			name:           "no authenticated hostnames",
			opts:           &SetupGitOptions{},
			expectedErr:    "SilentError",
			expectedErrOut: "You are not logged into any GitHub hosts. Run gh auth login to authenticate.\n",
		},
		{
			name: "not authenticated with the hostname given as flag",
			opts: &SetupGitOptions{
				Hostname: "foo",
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					require.NoError(t, cfg.Set("bar", "", ""))
					return cfg, nil
				},
			},
			expectedErr:    "You are not logged into the GitHub host \"foo\"\n",
			expectedErrOut: "",
		},
		{
			name: "error setting up git for hostname",
			opts: &SetupGitOptions{
				gitConfigure: &mockGitConfigurer{
					setupErr: fmt.Errorf("broken"),
				},
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					require.NoError(t, cfg.Set("bar", "", ""))
					return cfg, nil
				},
			},
			expectedErr:    "failed to set up git credential helper: broken",
			expectedErrOut: "",
		},
		{
			name: "no hostname option given. Setup git for each hostname in config",
			opts: &SetupGitOptions{
				gitConfigure: &mockGitConfigurer{},
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					require.NoError(t, cfg.Set("bar", "", ""))
					return cfg, nil
				},
			},
		},
		{
			name: "setup git for the hostname given via options",
			opts: &SetupGitOptions{
				Hostname:     "yes",
				gitConfigure: &mockGitConfigurer{},
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					require.NoError(t, cfg.Set("bar", "", ""))
					require.NoError(t, cfg.Set("yes", "", ""))
					return cfg, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.Config == nil {
				tt.opts.Config = func() (config.Config, error) {
					return config.NewBlankConfig(), nil
				}
			}

			io, _, _, stderr := iostreams.Test()

			io.SetStdinTTY(true)
			io.SetStderrTTY(true)
			io.SetStdoutTTY(true)
			tt.opts.IO = io

			err := setupGitRun(tt.opts)
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedErrOut, stderr.String())
		})
	}
}
