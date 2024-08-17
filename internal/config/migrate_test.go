package config

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestMigrationAppliedSuccessfully(t *testing.T) {
	readConfig := StubWriteConfig(t)

	// Given we have a migrator that writes some keys to the top level config
	// and hosts key
	c := ghConfig.ReadFromString(testFullConfig())

	topLevelKey := []string{"toplevelkey"}
	newHostKey := []string{hostsKey, "newhost"}

	migration := mockMigration(func(config *ghConfig.Config) error {
		config.Set(topLevelKey, "toplevelvalue")
		config.Set(newHostKey, "newhostvalue")
		return nil
	})

	// When we run the migration
	conf := cfg{c}
	require.NoError(t, conf.Migrate(migration))

	// Then our original config is updated with the migration applied
	requireKeyWithValue(t, c, topLevelKey, "toplevelvalue")
	requireKeyWithValue(t, c, newHostKey, "newhostvalue")

	// And our config / hosts changes are persisted to their relevant files
	// Note that this is real janky. We have writers that represent the
	// top level config and the hosts key but we don't merge them back together
	// so when we look into the hosts data, we don't nest the key we're
	// looking for under the hosts key ¯\_(ツ)_/¯
	var configBuf bytes.Buffer
	var hostsBuf bytes.Buffer
	readConfig(&configBuf, &hostsBuf)
	persistedCfg := ghConfig.ReadFromString(configBuf.String())
	persistedHosts := ghConfig.ReadFromString(hostsBuf.String())

	requireKeyWithValue(t, persistedCfg, topLevelKey, "toplevelvalue")
	requireKeyWithValue(t, persistedHosts, []string{"newhost"}, "newhostvalue")
}

func TestMigrationAppliedBumpsVersion(t *testing.T) {
	readConfig := StubWriteConfig(t)

	// Given we have a migration with a pre version that matches
	// the version in the config
	c := ghConfig.ReadFromString(testFullConfig())
	c.Set([]string{versionKey}, "expected-pre-version")
	topLevelKey := []string{"toplevelkey"}

	migration := &ghmock.MigrationMock{
		DoFunc: func(config *ghConfig.Config) error {
			config.Set(topLevelKey, "toplevelvalue")
			return nil
		},
		PreVersionFunc: func() string {
			return "expected-pre-version"
		},
		PostVersionFunc: func() string {
			return "expected-post-version"
		},
	}

	// When we migrate
	conf := cfg{c}
	require.NoError(t, conf.Migrate(migration))

	// Then our original config is updated with the migration applied
	requireKeyWithValue(t, c, topLevelKey, "toplevelvalue")
	requireKeyWithValue(t, c, []string{versionKey}, "expected-post-version")

	// And our config / hosts changes are persisted to their relevant files
	var configBuf bytes.Buffer
	readConfig(&configBuf, io.Discard)
	persistedCfg := ghConfig.ReadFromString(configBuf.String())

	requireKeyWithValue(t, persistedCfg, topLevelKey, "toplevelvalue")
	requireKeyWithValue(t, persistedCfg, []string{versionKey}, "expected-post-version")
}

func TestMigrationIsNoopWhenAlreadyApplied(t *testing.T) {
	// Given we have a migration with a post version that matches
	// the version in the config
	c := ghConfig.ReadFromString(testFullConfig())
	c.Set([]string{versionKey}, "expected-post-version")

	migration := &ghmock.MigrationMock{
		DoFunc: func(config *ghConfig.Config) error {
			return errors.New("is not called")
		},
		PreVersionFunc: func() string {
			return "is not called"
		},
		PostVersionFunc: func() string {
			return "expected-post-version"
		},
	}

	// When we run Migrate
	conf := cfg{c}
	err := conf.Migrate(migration)

	// Then there is nothing done and the config is not modified
	require.NoError(t, err)
	requireKeyWithValue(t, c, []string{versionKey}, "expected-post-version")
}

func TestMigrationErrorsWhenPreVersionMismatch(t *testing.T) {
	StubWriteConfig(t)

	// Given we have a migration with a pre version that does not match
	// the version in the config
	c := ghConfig.ReadFromString(testFullConfig())
	c.Set([]string{versionKey}, "not-expected-pre-version")
	topLevelKey := []string{"toplevelkey"}

	migration := &ghmock.MigrationMock{
		DoFunc: func(config *ghConfig.Config) error {
			config.Set(topLevelKey, "toplevelvalue")
			return nil
		},
		PreVersionFunc: func() string {
			return "expected-pre-version"
		},
		PostVersionFunc: func() string {
			return "not-expected"
		},
	}

	// When we run Migrate
	conf := cfg{c}
	err := conf.Migrate(migration)

	// Then there is an error the migration is not applied and the version is not modified
	require.ErrorContains(t, err, `failed to migrate as "expected-pre-version" pre migration version did not match config version "not-expected-pre-version"`)
	requireNoKey(t, c, topLevelKey)
	requireKeyWithValue(t, c, []string{versionKey}, "not-expected-pre-version")
}

func TestMigrationErrorWritesNoFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tempDir)

	// Given we have a migrator that errors
	c := ghConfig.ReadFromString(testFullConfig())
	migration := mockMigration(func(config *ghConfig.Config) error {
		return errors.New("failed to migrate in test")
	})

	// When we run the migration
	conf := cfg{c}
	err := conf.Migrate(migration)

	// Then the error is wrapped and bubbled
	require.EqualError(t, err, "failed to migrate config: failed to migrate in test")

	// And no files are written to disk
	files, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	require.Len(t, files, 0)
}

func TestMigrationWriteErrors(t *testing.T) {
	tests := []struct {
		name            string
		unwriteableFile string
		wantErrContains string
	}{
		{
			name:            "failure to write hosts",
			unwriteableFile: "hosts.yml",
			wantErrContains: "failed to write config after migration",
		},
		{
			name:            "failure to write config",
			unwriteableFile: "config.yml",
			wantErrContains: "failed to write config after migration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			t.Setenv("GH_CONFIG_DIR", tempDir)

			// Given we error when writing the files (because we chmod the files as trickery)
			makeFileUnwriteable(t, filepath.Join(tempDir, tt.unwriteableFile))

			c := ghConfig.ReadFromString(testFullConfig())
			topLevelKey := []string{"toplevelkey"}
			hostsKey := []string{hostsKey, "newhost"}

			migration := mockMigration(func(someCfg *ghConfig.Config) error {
				someCfg.Set(topLevelKey, "toplevelvalue")
				someCfg.Set(hostsKey, "newhostvalue")
				return nil
			})

			// When we run the migration
			conf := cfg{c}
			err := conf.Migrate(migration)

			// Then the error is wrapped and bubbled
			require.ErrorContains(t, err, tt.wantErrContains)
		})
	}
}

func makeFileUnwriteable(t *testing.T, file string) {
	t.Helper()

	f, err := os.Create(file)
	require.NoError(t, err)
	f.Close()

	require.NoError(t, os.Chmod(file, 0000))
}

func mockMigration(doFunc func(config *ghConfig.Config) error) *ghmock.MigrationMock {
	return &ghmock.MigrationMock{
		DoFunc: doFunc,
		PreVersionFunc: func() string {
			return ""
		},
		PostVersionFunc: func() string {
			return "not-expected"
		},
	}

}

func testFullConfig() string {
	var data = `
git_protocol: ssh
editor:
prompt: enabled
pager: less
hosts:
  github.com:
    user: user1
    oauth_token: xxxxxxxxxxxxxxxxxxxx
    git_protocol: ssh
  enterprise.com:
    user: user2
    oauth_token: yyyyyyyyyyyyyyyyyyyy
    git_protocol: https
`
	return data
}
