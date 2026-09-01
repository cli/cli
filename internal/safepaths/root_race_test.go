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

	err = root.WriteFile(filepath.Join("nested", "file.txt"), []byte("content"), 0o644, false)
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(redirectedDir, "file.txt"))
	assert.NoFileExists(t, filepath.Join(rootDir, "moved", "file.txt"))
}

func TestRootRenameUsesPinnedParentDuringSwap(t *testing.T) {
	rootDir := t.TempDir()
	parentDir := filepath.Join(rootDir, "nested")
	require.NoError(t, os.Mkdir(parentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(parentDir, "source"), []byte("original"), 0o644))
	redirectedDir := filepath.Join(rootDir, "redirected")
	require.NoError(t, os.Mkdir(redirectedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(redirectedDir, "source"), []byte("redirected"), 0o644))

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()

	defaultRename := root.rename
	root.rename = func(parent *os.Root, oldName, newName string) error {
		moved := filepath.Join(rootDir, "moved")
		require.NoError(t, os.Rename(parentDir, moved))
		if err := os.Symlink("redirected", parentDir); err != nil {
			t.Skipf("directory symlinks unavailable: %v", err)
		}
		return defaultRename(parent, oldName, newName)
	}

	require.NoError(t, root.Rename(filepath.Join("nested", "source"), filepath.Join("nested", "dest")))
	assert.FileExists(t, filepath.Join(rootDir, "moved", "dest"))
	assert.NoFileExists(t, filepath.Join(redirectedDir, "dest"))
	redirected, err := os.ReadFile(filepath.Join(redirectedDir, "source"))
	require.NoError(t, err)
	assert.Equal(t, "redirected", string(redirected))
}

func TestRootRenameRejectsCrossParent(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "one"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "two"), 0o755))
	source := filepath.Join(rootDir, "one", "source")
	require.NoError(t, os.WriteFile(source, []byte("content"), 0o644))

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()

	err = root.Rename(filepath.Join("one", "source"), filepath.Join("two", "dest"))
	require.ErrorContains(t, err, "common parent")
	content, err := os.ReadFile(source)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
	assert.NoFileExists(t, filepath.Join(rootDir, "two", "dest"))
}
