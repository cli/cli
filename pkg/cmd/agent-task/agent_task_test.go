package agent

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

// setupMockOAuthConfig configures a blank config with a default host and optional token behavior.
func setupMockOAuthConfig(t *testing.T, tokenSource string) gh.Config {
	t.Helper()
	c := config.NewMockConfig()
	switch tokenSource {
	case "oauth_token":
		// valid OAuth device flow token stored in config
		c.Set("github.com", "oauth_token", "gho_OAUTH123")
	case "keyring":
		// valid OAuth device flow token stored in keyring
		c.Set("github.com", "oauth_token", "gho_OAUTH123")
	case "GH_TOKEN":
		// classic style token stored in config (will fail prefix check)
		c.Set("github.com", "oauth_token", "ghp_CLASSIC123")
	case "GH_ENTERPRISE_TOKEN":
		// enterprise style token stored in config (will fail prefix check)
		c.Set("something.ghes.com", "oauth_token", "ghe_ENTERPRISE123")
	}
	return c
}

func TestNewCmdAgentTask(t *testing.T) {
	tests := []struct {
		name            string
		tokenSource     string
		customConfig    func() (gh.Config, error)
		wantErr         bool
		wantErrContains string
		wantStdout      string
	}{
		{
			name:        "oauth token is accepted",
			tokenSource: "oauth_token",
			wantErr:     false,
			wantStdout:  "",
		},
		{
			name:        "keyring oauth token is accepted",
			tokenSource: "keyring",
			wantErr:     false,
			wantStdout:  "",
		},
		{
			name:            "env var token is rejected",
			tokenSource:     "GH_TOKEN",
			wantErr:         true,
			wantErrContains: "requires an OAuth token",
		},
		{
			name:        "enterprise token alone is ignored and rejected",
			tokenSource: "GH_ENTERPRISE_TOKEN",
			wantErr:     true,
		},
		{
			name: "github.com oauth is accepted and enterprise token ignored",
			customConfig: func() (gh.Config, error) {
				c := config.NewMockConfig()
				c.Set("something.ghes.com", "oauth_token", "ghe_ENTERPRISE123")
				c.Set("github.com", "oauth_token", "gho_OAUTH123")
				return c, nil
			},
			wantErr:    false,
			wantStdout: "",
		},
		{
			name: "enterprise host is rejected",
			customConfig: func() (gh.Config, error) {
				return &ghmock.ConfigMock{
					AuthenticationFunc: func() gh.AuthConfig {
						c := &config.AuthConfig{}
						c.SetDefaultHost("something.ghes.com", "GH_HOST")
						return c
					},
				}, nil
			},
			wantErr:         true,
			wantErrContains: "not supported on this host",
		},
		{
			name: "empty host is rejected",
			customConfig: func() (gh.Config, error) {
				return &ghmock.ConfigMock{
					AuthenticationFunc: func() gh.AuthConfig {
						c := &config.AuthConfig{}
						c.SetDefaultHost("", "GH_HOST")
						return c
					},
				}, nil
			},
			wantErr:         true,
			wantErrContains: "no default host configured",
		},
		{
			name:        "no auth is rejected",
			tokenSource: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}
			ios, _, stdout, _ := iostreams.Test()
			f.IOStreams = ios
			if tt.customConfig != nil {
				f.Config = tt.customConfig
			} else {
				f.Config = func() (gh.Config, error) { return setupMockOAuthConfig(t, tt.tokenSource), nil }
			}

			cmd := NewCmdAgentTask(f)
			err := cmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContains != "" {
					require.Contains(t, err.Error(), tt.wantErrContains)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantStdout, stdout.String())
			}
		})
	}
}

func TestAliasAreSet(t *testing.T) {
	f := &cmdutil.Factory{}
	ios, _, _, _ := iostreams.Test()
	f.IOStreams = ios
	f.Config = func() (gh.Config, error) { return setupMockOAuthConfig(t, "oauth_token"), nil }

	cmd := NewCmdAgentTask(f)

	require.ElementsMatch(t, []string{"agent-tasks", "agent", "agents"}, cmd.Aliases)
}
