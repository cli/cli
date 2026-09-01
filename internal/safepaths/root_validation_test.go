package safepaths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTempDirFailurePaths(t *testing.T) {
	tests := []struct {
		name      string
		configure func(root *Root)
		wantErr   string
		verify    func(t *testing.T, root *Root)
	}{
		{
			name: "random source error",
			configure: func(root *Root) {
				root.random = func([]byte) (int, error) {
					return 0, errors.New("random failed")
				}
			},
			wantErr: "random failed",
		},
		{
			name: "mkdir error",
			configure: func(root *Root) {
				root.mkdir = func(*os.Root, string, os.FileMode) error {
					return errors.New("mkdir failed")
				}
			},
			wantErr: "mkdir failed",
		},
		{
			name: "name exhaustion",
			configure: func(root *Root) {
				root.mkdir = func(*os.Root, string, os.FileMode) error {
					return os.ErrExist
				}
			},
			wantErr: "could not create a unique temporary directory",
		},
		{
			name: "opening created directory fails",
			configure: func(root *Root) {
				root.openRoot = func(*os.Root, string) (*os.Root, error) {
					return nil, errors.New("open failed")
				}
			},
			wantErr: "open failed",
			verify: func(t *testing.T, root *Root) {
				t.Helper()
				entries, err := os.ReadDir(root.String())
				require.NoError(t, err)
				assert.Empty(t, entries)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root, err := OpenRoot(t.TempDir())
			require.NoError(t, err)
			defer root.Close()
			tt.configure(root)

			name, tempRoot, err := root.TempDir(".temp-")
			assert.Empty(t, name)
			assert.Nil(t, tempRoot)
			require.ErrorContains(t, err, tt.wantErr)
			if tt.verify != nil {
				tt.verify(t, root)
			}
		})
	}
}

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

	file, err := root.Open("expected")
	assert.Nil(t, file)
	require.ErrorContains(t, err, "changed while opening")
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

	file, err := root.Create("link", 0o644, true)
	assert.Nil(t, file)
	require.ErrorContains(t, err, "could not safely classify redirect target")
	info, err := os.Lstat(filepath.Join(rootDir, "link"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
}
