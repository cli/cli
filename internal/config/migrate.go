package config

import (
	"fmt"

	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

//go:generate moq -rm -out migration_mock.go . Migration

// Migration is the interace that config migrations must implement.
//
// Migrations will receive a copy of the config, and should modify that copy
// as necessary. After migration has completed, the modified config contents
// will be used.
//
// The calling code is expected to verify that the current version of the config
// matches the PreVersion of the migration before calling Do, and will set the
// config version to the PostVersion after the migration has completed successfully.
type Migration interface {
	// PreVersion is the required config version for this to be applied
	PreVersion() string
	// PostVersion is the config version that must be applied after migration
	PostVersion() string
	// Do is expected to apply any necessary changes to the config in place
	Do(*ghConfig.Config) error
}

func Migrate(c *ghConfig.Config, m Migration) error {
	// It is expected initially that there is no version key because we don't
	// have one to begin with, so an error is expected.
	version, _ := c.Get([]string{versionKey})
	if m.PreVersion() != version {
		return fmt.Errorf("failed to migrate as %q pre migration version did not match config version %q", m.PreVersion(), version)
	}

	if err := m.Do(c); err != nil {
		return fmt.Errorf("failed to migrate config: %s", err)
	}

	c.Set([]string{versionKey}, m.PostVersion())

	// Then write out our migrated config.
	if err := ghConfig.Write(c); err != nil {
		return fmt.Errorf("failed to write config after migration: %s", err)
	}

	return nil
}
