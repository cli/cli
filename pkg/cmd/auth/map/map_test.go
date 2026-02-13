package authmap

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestParseScope(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		hostname    string
		defaultHost string
		want        mapScope
		wantErr     string
	}{
		{
			name:        "owner wildcard with default host",
			scope:       "devolutions/*",
			defaultHost: "github.com",
			want: mapScope{
				hostname: "github.com",
				owner:    "devolutions",
				ownerAll: true,
			},
		},
		{
			name:  "exact repo with explicit host",
			scope: "github.com/awakecoding/msrdpex",
			want: mapScope{
				hostname: "github.com",
				owner:    "awakecoding",
				repo:     "msrdpex",
			},
		},
		{
			name:        "invalid scope",
			scope:       "invalid",
			defaultHost: "github.com",
			wantErr:     "invalid scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScope(tt.scope, tt.hostname, tt.defaultHost)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSetListAndRemoveRun(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	cfg, _ := config.NewIsolatedTestConfig(t)

	authCfg := cfg.Authentication()
	_, err := authCfg.Login("github.com", "work-user", "work-token", "", false)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "personal-user", "personal-token", "", false)
	require.NoError(t, err)

	configFn := func() (gh.Config, error) {
		return cfg, nil
	}

	err = setRun(&SetOptions{
		IO:       ios,
		Config:   configFn,
		Scope:    "devolutions/*",
		Username: "work-user",
	})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "github.com/devolutions/* => work-user")

	stdout.Reset()
	err = listRun(&ListOptions{
		IO:     ios,
		Config: configFn,
	})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "devolutions/* => work-user")

	stdout.Reset()
	err = removeRun(&RemoveOptions{
		IO:     ios,
		Config: configFn,
		Scope:  "devolutions/*",
	})
	require.NoError(t, err)

	stdout.Reset()
	err = listRun(&ListOptions{
		IO:       ios,
		Config:   configFn,
		Hostname: "github.com",
	})
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "No repository account mappings configured")
}
