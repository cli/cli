package safepaths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootRejectsDirectorySwapDuringOpen(t *testing.T) {
	rootDir := t.TempDir()
	originalDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.Mkdir(originalDir, 0o755))
	redirectedDir := filepath.Join(rootDir, "redirected")
	require.NoError(t, os.Mkdir(redirectedDir, 0o755))

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()

	defaultOpenRoot := root.openRoot
	swapped := false
	root.openRoot = func(parent *os.Root, name string) (*os.Root, error) {
		if name == "nested" && !swapped {
			swapped = true
			moved := filepath.Join(rootDir, "moved")
			require.NoError(t, os.Rename(originalDir, moved))
			if err := os.Symlink("redirected", originalDir); err != nil {
				t.Skipf("directory symlinks unavailable: %v", err)
			}
		}
		return defaultOpenRoot(parent, name)
	}

	err = root.WriteFile(filepath.Join("nested", "file.txt"), []byte("content"), 0o644, 0o755, false)
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(redirectedDir, "file.txt"))
	assert.NoFileExists(t, filepath.Join(rootDir, "moved", "file.txt"))
}
