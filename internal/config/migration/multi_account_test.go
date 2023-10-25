package migration_test

import (
	"testing"

	"github.com/cli/cli/v2/internal/config/migration"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func TestMigration(t *testing.T) {
	cfg := config.ReadFromString(`
hosts:
  github.com:
    user: user1
    oauth_token: xxxxxxxxxxxxxxxxxxxx
    git_protocol: ssh
  enterprise.com:
    user: user2
    oauth_token: yyyyyyyyyyyyyyyyyyyy
    git_protocol: https
`)

	var m migration.MultiAccount
	require.NoError(t, m.Do(cfg))

	// Do some simple checks here for depth and multiple migrations
	// but I don't really want to write a full tree traversal matcher.

	// First we'll check that the data has been copied to the new structure
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "users", "user1", "git_protocol"}, "ssh")
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "users", "user1", "oauth_token"}, "xxxxxxxxxxxxxxxxxxxx")

	requireKeyWithValue(t, cfg, []string{"hosts", "enterprise.com", "users", "user2", "git_protocol"}, "https")
	requireKeyWithValue(t, cfg, []string{"hosts", "enterprise.com", "users", "user2", "oauth_token"}, "yyyyyyyyyyyyyyyyyyyy")

	// Then we'll check that the old data has been left alone
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "user"}, "user1")
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "oauth_token"}, "xxxxxxxxxxxxxxxxxxxx")
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "git_protocol"}, "ssh")

	requireKeyWithValue(t, cfg, []string{"hosts", "enterprise.com", "user"}, "user2")
	requireKeyWithValue(t, cfg, []string{"hosts", "enterprise.com", "oauth_token"}, "yyyyyyyyyyyyyyyyyyyy")
	requireKeyWithValue(t, cfg, []string{"hosts", "enterprise.com", "git_protocol"}, "https")
}

func TestPreVersionIsEmptyString(t *testing.T) {
	var m migration.MultiAccount
	require.Equal(t, "", m.PreVersion())
}

func TestPostVersion(t *testing.T) {
	var m migration.MultiAccount
	require.Equal(t, "1", m.PostVersion())
}

func TestMigrationErrorsWithDeeplyNestedEntries(t *testing.T) {
	cfg := config.ReadFromString(`
hosts:
  github.com:
    user: user1
    nested:
      too: deep
`)

	var m migration.MultiAccount
	err := m.Do(cfg)

	require.ErrorContains(t, err, "hosts file has entries that are surprisingly deeply nested")
}

func TestMigrationReturnsSuccessfullyWhenNoHostsEntry(t *testing.T) {
	cfg := config.ReadFromString(``)

	var m migration.MultiAccount
	require.NoError(t, m.Do(cfg))
}

func TestMigrationReturnsSuccessfullyWhenEmptyHosts(t *testing.T) {
	cfg := config.ReadFromString(`
hosts:
`)

	var m migration.MultiAccount
	require.NoError(t, m.Do(cfg))
}

func TestMigrationReturnsSuccessfullyWhenAnonymousUserExists(t *testing.T) {
	// Simulates config that gets generated when a user logs
	// in with a token and git protocol is not specified and
	// secure storage is used.
	keyring.MockInit()
	require.NoError(t, keyring.Set("gh:github.com", "", "test-token"))

	cfg := config.ReadFromString(`
hosts:
  github.com:
    user: x-access-token
`)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.GraphQL(`query CurrentUser\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`),
	)

	m := migration.MultiAccount{Transport: reg}
	require.NoError(t, m.Do(cfg))

	require.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "user"}, "monalisa")
	// monalisa key gets created with no value
	users, err := cfg.Keys([]string{"hosts", "github.com", "users"})
	require.NoError(t, err)
	require.Equal(t, []string{"monalisa"}, users)
}

func TestMigrationReturnsSuccessfullyWhenAnonymousUserExistsAndGitProtocol(t *testing.T) {
	// Simulates config that gets generated when a user logs
	// in with a token and git protocol is specified and
	// secure storage is used.
	keyring.MockInit()
	require.NoError(t, keyring.Set("gh:github.com", "", "test-token"))

	cfg := config.ReadFromString(`
hosts:
  github.com:
    user: x-access-token
    git_protocol: ssh
`)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.GraphQL(`query CurrentUser\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`),
	)

	m := migration.MultiAccount{Transport: reg}
	require.NoError(t, m.Do(cfg))

	require.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "user"}, "monalisa")
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "users", "monalisa", "git_protocol"}, "ssh")
}

func TestMigrationReturnsSuccessfullyWhenAnonymousUserExistsAndInsecureStorage(t *testing.T) {
	// Simulates config that gets generated when a user logs
	// in with a token and git protocol is specified and
	// secure storage is not used.
	cfg := config.ReadFromString(`
hosts:
  github.com:
    user: x-access-token
    oauth_token: test-token
    git_protocol: ssh
`)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.GraphQL(`query CurrentUser\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`),
	)

	m := migration.MultiAccount{Transport: reg}
	require.NoError(t, m.Do(cfg))

	require.Equal(t, "token test-token", reg.Requests[0].Header.Get("Authorization"))
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "user"}, "monalisa")
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "users", "monalisa", "oauth_token"}, "test-token")
	requireKeyWithValue(t, cfg, []string{"hosts", "github.com", "users", "monalisa", "git_protocol"}, "ssh")
}

func requireKeyWithValue(t *testing.T, cfg *config.Config, keys []string, value string) {
	t.Helper()

	actual, err := cfg.Get(keys)
	require.NoError(t, err)

	require.Equal(t, value, actual)
}
