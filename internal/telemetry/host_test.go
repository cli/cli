package telemetry_test

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCmd builds a "gh <parent> <name>" command tree for testing. The returned
// command is the leaf that callers interact with.
func newCmd(t *testing.T, parentName, name string, withRepoFlag, withHostnameFlag bool) *cobra.Command {
	t.Helper()
	leaf := &cobra.Command{Use: name}
	if withRepoFlag {
		leaf.Flags().String("repo", "", "")
	}
	if withHostnameFlag {
		leaf.Flags().String("hostname", "", "")
	}
	root := &cobra.Command{Use: "gh"}
	if parentName != "" {
		parent := &cobra.Command{Use: parentName}
		root.AddCommand(parent)
		parent.AddCommand(leaf)
	} else {
		root.AddCommand(leaf)
	}
	return leaf
}

func TestGuessTargetHost(t *testing.T) {
	tests := []struct {
		name              string
		parentName        string
		cmdName           string
		withRepoFlag      bool
		withHostnameFlag  bool
		repoFlagValue     string
		hostnameFlagValue string
		parseArgs         []string
		ghRepoEnv         string
		baseRepo          func() (ghrepo.Interface, error)
		config            func() (gh.Config, error)
		want              string
	}{
		{
			name:          "returns host from --repo flag",
			parentName:    "pr",
			cmdName:       "view",
			withRepoFlag:  true,
			repoFlagValue: "cli/cli",
			want:          "github.com",
		},
		{
			name:          "returns host from --repo flag with custom host",
			parentName:    "pr",
			cmdName:       "view",
			withRepoFlag:  true,
			repoFlagValue: "ghe.example.com/cli/cli",
			want:          "ghe.example.com",
		},
		{
			name:              "returns host from --hostname flag",
			parentName:        "auth",
			cmdName:           "login",
			withHostnameFlag:  true,
			hostnameFlagValue: "ghe.example.com",
			want:              "ghe.example.com",
		},
		{
			name:              "--repo takes priority over --hostname",
			parentName:        "pr",
			cmdName:           "view",
			withRepoFlag:      true,
			withHostnameFlag:  true,
			repoFlagValue:     "cli/cli",
			hostnameFlagValue: "ghe.example.com",
			want:              "github.com",
		},
		{
			name:       "returns host from positional arg under gh repo",
			parentName: "repo",
			cmdName:    "view",
			parseArgs:  []string{"ghe.example.com/owner/repo"},
			want:       "ghe.example.com",
		},
		{
			name:       "does not use positional arg outside of gh repo parent",
			parentName: "pr",
			cmdName:    "view",
			parseArgs:  []string{"ghe.example.com/owner/repo"},
			want:       "",
		},
		{
			name:       "returns host from GH_REPO env",
			parentName: "pr",
			cmdName:    "view",
			ghRepoEnv:  "ghe.example.com/owner/repo",
			want:       "ghe.example.com",
		},
		{
			name:          "flag takes priority over GH_REPO env",
			parentName:    "pr",
			cmdName:       "view",
			withRepoFlag:  true,
			repoFlagValue: "cli/cli",
			ghRepoEnv:     "ghe.example.com/owner/repo",
			want:          "github.com",
		},
		{
			name:         "falls back to BaseRepo from factory for repo-scoped commands",
			parentName:   "pr",
			cmdName:      "view",
			withRepoFlag: true,
			baseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.NewWithHost("owner", "repo", "ghe.example.com"), nil
			},
			want: "ghe.example.com",
		},
		{
			name:       "skips BaseRepo for commands without --repo flag",
			parentName: "pr",
			cmdName:    "view",
			baseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.NewWithHost("owner", "repo", "ghe.example.com"), nil
			},
			want: "",
		},
		{
			name:         "ignores BaseRepo when it returns an error",
			parentName:   "pr",
			cmdName:      "view",
			withRepoFlag: true,
			baseRepo: func() (ghrepo.Interface, error) {
				return nil, errors.New("no base repo")
			},
			want: "",
		},
		{
			name:       "falls back to default host from config",
			parentName: "pr",
			cmdName:    "view",
			config: func() (gh.Config, error) {
				return &ghmock.ConfigMock{
					AuthenticationFunc: func() gh.AuthConfig {
						return &stubAuthConfig{defaultHost: "ghe.example.com"}
					},
				}, nil
			},
			want: "ghe.example.com",
		},
		{
			name:       "returns empty string when no signal is available",
			parentName: "pr",
			cmdName:    "view",
			want:       "",
		},
		{
			name:       "returns empty string when config returns an error",
			parentName: "pr",
			cmdName:    "view",
			config:     func() (gh.Config, error) { return nil, errors.New("no config") },
			want:       "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_REPO", tt.ghRepoEnv)

			cmd := newCmd(t, tt.parentName, tt.cmdName, tt.withRepoFlag, tt.withHostnameFlag)
			if tt.repoFlagValue != "" {
				require.NoError(t, cmd.Flags().Set("repo", tt.repoFlagValue))
			}
			if tt.hostnameFlagValue != "" {
				require.NoError(t, cmd.Flags().Set("hostname", tt.hostnameFlagValue))
			}
			if tt.parseArgs != nil {
				require.NoError(t, cmd.ParseFlags(tt.parseArgs))
			}

			got := telemetry.GuessTargetHost(cmd, tt.baseRepo, tt.config)
			assert.Equal(t, tt.want, got)
		})
	}
}

// stubAuthConfig is a minimal gh.AuthConfig implementation for tests that only
// exercise the default host lookup. The production gh.AuthConfig is a concrete
// struct with unexported fields so we use the interface that Authentication()
// returns instead.
type stubAuthConfig struct {
	gh.AuthConfig
	defaultHost string
}

func (s *stubAuthConfig) DefaultHost() (string, string) {
	return s.defaultHost, "default"
}
