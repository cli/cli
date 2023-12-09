package setupgit

import (
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

type mockGitConfigurer struct {
	hosts    []string
	setupErr error
}

func (gf *mockGitConfigurer) SetupFor(hostname string) []string {
	return gf.hosts
}

func (gf *mockGitConfigurer) Setup(hostname, username, authToken string) error {
	gf.hosts = append(gf.hosts, hostname)
	return gf.setupErr
}

func Test_setupGitRun(t *testing.T) {
	tests := []struct {
		name               string
		opts               *SetupGitOptions
		setupErr           error
		cfgStubs           func(*testing.T, config.Config)
		expectedHostsSetup []string
		expectedErr        string
		expectedErrOut     string
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
			},
			cfgStubs: func(t *testing.T, cfg config.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			expectedErr:    "You are not logged into the GitHub host \"foo\"\n",
			expectedErrOut: "",
		},
		{
			name:     "error setting up git for hostname",
			opts:     &SetupGitOptions{},
			setupErr: fmt.Errorf("broken"),
			cfgStubs: func(t *testing.T, cfg config.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			expectedErr:    "failed to set up git credential helper: broken",
			expectedErrOut: "",
		},
		{
			name: "no hostname option given. Setup git for each hostname in config",
			opts: &SetupGitOptions{},
			cfgStubs: func(t *testing.T, cfg config.Config) {
				login(t, cfg, "ghe.io", "test-user", "gho_ABCDEFG", "https", false)
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			expectedHostsSetup: []string{"github.com", "ghe.io"},
		},
		{
			name: "setup git for the hostname given via options",
			opts: &SetupGitOptions{
				Hostname: "ghe.io",
			},
			cfgStubs: func(t *testing.T, cfg config.Config) {
				login(t, cfg, "ghe.io", "test-user", "gho_ABCDEFG", "https", false)
			},
			expectedHostsSetup: []string{"ghe.io"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stderr := iostreams.Test()

			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			cfg, _ := config.NewIsolatedTestConfig(t)
			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}

			if tt.opts.Config == nil {
				tt.opts.Config = func() (config.Config, error) {
					return cfg, nil
				}
			}

			gcSpy := &mockGitConfigurer{setupErr: tt.setupErr}
			tt.opts.gitConfigure = gcSpy

			err := setupGitRun(tt.opts)
			if tt.expectedErr != "" {
				require.EqualError(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
			}

			if tt.expectedHostsSetup != nil {
				require.Equal(t, tt.expectedHostsSetup, gcSpy.hosts)
			}

			require.Equal(t, tt.expectedErrOut, stderr.String())
		})
	}
}

func login(t *testing.T, c config.Config, hostname, username, token, gitProtocol string, secureStorage bool) {
	t.Helper()
	_, err := c.Authentication().Login(hostname, username, token, "https", secureStorage)
	require.NoError(t, err)
}
