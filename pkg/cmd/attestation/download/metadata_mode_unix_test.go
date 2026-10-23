//go:build unix

package download

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateMetadataFileMode(t *testing.T) {
	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)

	store := NewLiveStore(t.TempDir())
	path, err := store.createMetadataFile("sha256:value", nil)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o666), info.Mode().Perm())

	require.NoError(t, os.Chmod(path, 0o600))
	_, err = store.createMetadataFile("sha256:value", nil)
	require.NoError(t, err)
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
