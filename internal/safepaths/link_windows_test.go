package safepaths

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReparsePointClassification(t *testing.T) {
	t.Run("name surrogate redirects", func(t *testing.T) {
		info := reparsePointInfo{mode: fs.ModeSymlink}
		redirect, err := classifyWindowsPathRedirect(info, func() error {
			return errors.New("should not be called")
		})
		require.NoError(t, err)
		assert.True(t, redirect)
	})
	t.Run("mount point redirects", func(t *testing.T) {
		info := reparsePointInfo{
			mode:       fs.ModeIrregular,
			attributes: syscall.FILE_ATTRIBUTE_REPARSE_POINT,
		}
		redirect, err := classifyWindowsPathRedirect(info, func() error { return nil })
		require.NoError(t, err)
		assert.True(t, redirect)
	})
	t.Run("non-link reparse point fails closed", func(t *testing.T) {
		info := reparsePointInfo{
			mode:       fs.ModeDir,
			attributes: syscall.FILE_ATTRIBUTE_REPARSE_POINT,
		}
		redirect, err := classifyWindowsPathRedirect(info, func() error { return os.ErrNotExist })
		assert.False(t, redirect)
		require.ErrorIs(t, err, errUnsupportedNonLinkReparse)
	})
	t.Run("unexpected readlink error is returned", func(t *testing.T) {
		info := reparsePointInfo{
			mode:       fs.ModeIrregular,
			attributes: syscall.FILE_ATTRIBUTE_REPARSE_POINT,
		}

		redirect, err := classifyWindowsPathRedirect(info, func() error {
			return errors.New("readlink failed")
		})
		assert.False(t, redirect)
		require.ErrorContains(t, err, "readlink failed")
	})
}

func TestCreateReplaceFailsClosedForNonLinkReparse(t *testing.T) {
	rootDir := t.TempDir()
	path := filepath.Join(rootDir, "entry")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))
	before, err := os.Stat(path)
	require.NoError(t, err)

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()
	root.lstat = func(*os.Root, string) (os.FileInfo, error) {
		return reparsePointInfo{
			mode:       fs.ModeIrregular,
			attributes: syscall.FILE_ATTRIBUTE_REPARSE_POINT,
		}, nil
	}
	root.readlink = func(*os.Root, string) (string, error) {
		return "", os.ErrNotExist
	}

	file, err := root.Create("entry", 0o644, 0o755, true)
	assert.Nil(t, file)
	require.ErrorIs(t, err, errUnsupportedNonLinkReparse)
	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
}

func TestRootRejectsJunction(t *testing.T) {
	rootDir := t.TempDir()
	target := t.TempDir()
	junction := filepath.Join(rootDir, "junction")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, target).CombinedOutput(); err != nil {
		t.Skipf("junctions unavailable: %v: %s", err, output)
	}

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()

	err = root.WriteFile(filepath.Join("junction", "file.txt"), []byte("content"), 0o644, 0o755, false)
	require.ErrorContains(t, err, "symbolic link")
	assert.NoFileExists(t, filepath.Join(target, "file.txt"))

	err = root.WriteFile("junction", []byte("content"), 0o644, 0o755, true)
	require.Error(t, err)
	info, err := os.Lstat(junction)
	require.NoError(t, err)
	assert.NotZero(t, info.Sys().(*syscall.Win32FileAttributeData).FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT)
}

func TestOpenFileRejectsWindowsTrailingSeparator(t *testing.T) {
	tests := []struct {
		name      string
		separator string
	}{
		{name: "backslash", separator: `\`},
		{name: "slash", separator: `/`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "output") + tt.separator

			file, err := OpenFile(path, 0o644, 0o755, false)
			if file != nil {
				_ = file.Close()
			}
			require.ErrorContains(t, err, "ends in a separator")
			_, statErr := os.Lstat(filepath.Join(root, "output"))
			require.True(t, os.IsNotExist(statErr))
		})
	}
}

func TestRootCreateRejectsWindowsTrailingSeparator(t *testing.T) {
	tests := []struct {
		name      string
		separator string
	}{
		{name: "backslash", separator: `\`},
		{name: "slash", separator: `/`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()
			root, err := OpenRoot(rootDir)
			require.NoError(t, err)
			defer root.Close()

			err = root.WriteFile("output"+tt.separator, []byte("content"), 0o644, 0o755, false)
			require.ErrorContains(t, err, "ends in a separator")
			_, statErr := os.Lstat(filepath.Join(rootDir, "output"))
			require.True(t, os.IsNotExist(statErr))
			assert.NoFileExists(t, filepath.Join(rootDir, "output", "output"))
		})
	}
}

type reparsePointInfo struct {
	mode       fs.FileMode
	attributes uint32
}

func (reparsePointInfo) Name() string        { return "reparse" }
func (reparsePointInfo) Size() int64         { return 0 }
func (i reparsePointInfo) Mode() fs.FileMode { return i.mode }
func (reparsePointInfo) ModTime() time.Time  { return time.Time{} }
func (i reparsePointInfo) IsDir() bool       { return i.mode.IsDir() }
func (i reparsePointInfo) Sys() any {
	return &syscall.Win32FileAttributeData{FileAttributes: i.attributes}
}
