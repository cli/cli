package safepaths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectoryIdentityMismatch(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "expected"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "other"), 0o755))

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()
	defaultOpenRoot := root.openRoot
	root.openRoot = func(parent *os.Root, name string) (*os.Root, error) {
		if name == "expected" {
			return defaultOpenRoot(parent, "other")
		}
		return defaultOpenRoot(parent, name)
	}

	child, err := root.Sub("expected", 0o755)
	assert.Nil(t, child)
	require.ErrorContains(t, err, "changed while opening")
}

func TestFileIdentityMismatch(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "expected"), []byte("expected"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "other"), []byte("other"), 0o644))

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()
	defaultOpenFile := root.openFile
	root.openFile = func(parent *os.Root, name string, flag int, perm os.FileMode) (*os.File, error) {
		if name == "expected" {
			return defaultOpenFile(parent, "other", flag, perm)
		}
		return defaultOpenFile(parent, name, flag, perm)
	}

	file, err := root.Create("expected", 0o644, 0o755, true)
	assert.Nil(t, file)
	require.ErrorContains(t, err, "changed while opening")
	content, err := os.ReadFile(filepath.Join(rootDir, "other"))
	require.NoError(t, err)
	assert.Equal(t, "other", string(content))
}

func TestRedirectTargetClassificationError(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "target"), []byte("target"), 0o644))
	require.NoError(t, os.Symlink("target", filepath.Join(rootDir, "link")))

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()
	root.stat = func(*os.Root, string) (os.FileInfo, error) {
		return nil, errors.New("stat failed")
	}

	file, err := root.Create("link", 0o644, 0o755, true)
	assert.Nil(t, file)
	require.ErrorContains(t, err, "could not safely classify redirect target")
	info, err := os.Lstat(filepath.Join(rootDir, "link"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}
