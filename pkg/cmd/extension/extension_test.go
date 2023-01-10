package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdateAvailable_IsLocal(t *testing.T) {
	e := &Extension{
		isLocal: true,
	}

	assert.False(t, e.UpdateAvailable())
}

func TestUpdateAvailable_NoCurrentVersion(t *testing.T) {
	e := &Extension{
		isLocal: false,
	}

	assert.False(t, e.UpdateAvailable())
}

func TestUpdateAvailable_NoLatestVersion(t *testing.T) {
	e := &Extension{
		isLocal:        false,
		currentVersion: "1.0.0",
	}

	assert.False(t, e.UpdateAvailable())
}

func TestUpdateAvailable_CurrentVersionIsLatestVersion(t *testing.T) {
	e := &Extension{
		isLocal:        false,
		currentVersion: "1.0.0",
		latestVersion:  "1.0.0",
	}

	assert.False(t, e.UpdateAvailable())
}

func TestUpdateAvailable(t *testing.T) {
	e := &Extension{
		isLocal:        false,
		currentVersion: "1.0.0",
		latestVersion:  "1.1.0",
	}

	assert.True(t, e.UpdateAvailable())
}
