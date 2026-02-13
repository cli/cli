package factory

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/stretchr/testify/require"
)

func TestWithRepositoryTokenMappingUsesMappedUserToken(t *testing.T) {
	cfg, _ := config.NewIsolatedTestConfig(t)
	authCfg := cfg.Authentication()

	_, err := authCfg.Login("github.com", "work-user", "work-token", "", false)
	require.NoError(t, err)
	_, err = authCfg.Login("github.com", "personal-user", "personal-token", "", false)
	require.NoError(t, err)
	require.NoError(t, authCfg.SwitchUser("github.com", "personal-user"))

	repoMapper := authCfg.(*config.AuthConfig)
	require.NoError(t, repoMapper.SetUserForOwner("github.com", "devolutions", "work-user"))

	getter := withRepositoryTokenMapping(authCfg, ghrepo.NewWithHost("devolutions", "github-cli", "github.com"))
	token, source := getter.ActiveToken("github.com")

	require.Equal(t, "work-token", token)
	require.Equal(t, "oauth_token", source)
}

func TestWithRepositoryTokenMappingFallsBackToDefaultActiveToken(t *testing.T) {
	cfg, _ := config.NewIsolatedTestConfig(t)
	authCfg := cfg.Authentication()

	_, err := authCfg.Login("github.com", "personal-user", "personal-token", "", false)
	require.NoError(t, err)

	getter := withRepositoryTokenMapping(authCfg, ghrepo.NewWithHost("devolutions", "github-cli", "github.com"))
	token, source := getter.ActiveToken("github.com")

	require.Equal(t, "personal-token", token)
	require.Equal(t, "oauth_token", source)
}

func TestWithRepositoryTokenMappingRespectsEnvTokenPrecedence(t *testing.T) {
	t.Setenv("GH_TOKEN", "env-token")

	cfg, _ := config.NewIsolatedTestConfig(t)
	authCfg := cfg.Authentication()

	_, err := authCfg.Login("github.com", "work-user", "work-token", "", false)
	require.NoError(t, err)

	repoMapper := authCfg.(*config.AuthConfig)
	require.NoError(t, repoMapper.SetUserForOwner("github.com", "devolutions", "work-user"))

	getter := withRepositoryTokenMapping(authCfg, ghrepo.NewWithHost("devolutions", "github-cli", "github.com"))
	token, source := getter.ActiveToken("github.com")

	require.Equal(t, "env-token", token)
	require.Equal(t, "GH_TOKEN", source)
}
