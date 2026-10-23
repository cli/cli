//go:build unix

package safepaths

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateReplacePreservesDevice(t *testing.T) {
	info, err := os.Lstat("/dev/null")
	if err != nil || info.Mode()&os.ModeDevice == 0 {
		t.Skip("/dev/null is unavailable")
	}

	root, err := OpenRoot("/dev")
	require.NoError(t, err)
	defer root.Close()
	file, err := root.Create("null", 0o644, 0o755, true)
	require.NoError(t, err)
	_, err = file.Write([]byte("content"))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	after, err := os.Lstat("/dev/null")
	require.NoError(t, err)
	assert.NotZero(t, after.Mode()&os.ModeDevice)
	assert.True(t, os.SameFile(info, after))
}

func TestCreateReplaceDoesNotUnlinkFIFO(t *testing.T) {
	rootDir := t.TempDir()
	fifo := filepath.Join(rootDir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	before, err := os.Lstat(fifo)
	require.NoError(t, err)

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()

	result := make(chan error, 1)
	go func() {
		file, err := root.Create("fifo", 0o644, 0o755, true)
		if file != nil {
			_ = file.Close()
		}
		result <- err
	}()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "named pipes cannot be overwritten safely")
	case <-time.After(time.Second):
		reader, openErr := os.OpenFile(fifo, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if openErr == nil {
			_ = reader.Close()
		}
		t.Fatal("FIFO replacement blocked without a reader")
	}

	after, err := os.Lstat(fifo)
	require.NoError(t, err)
	assert.NotZero(t, after.Mode()&os.ModeNamedPipe)
	assert.True(t, os.SameFile(before, after))
}

func TestCreateReplaceDoesNotBlockOnFIFOSwap(t *testing.T) {
	rootDir := t.TempDir()
	path := filepath.Join(rootDir, "target")
	require.NoError(t, os.WriteFile(path, []byte("regular"), 0o600))

	root, err := OpenRoot(rootDir)
	require.NoError(t, err)
	defer root.Close()
	defaultOpenFile := root.openFile
	root.openFile = func(parent *os.Root, name string, flag int, perm os.FileMode) (*os.File, error) {
		require.NoError(t, os.Remove(path))
		require.NoError(t, syscall.Mkfifo(path, 0o600))
		return defaultOpenFile(parent, name, flag, perm)
	}

	start := time.Now()
	file, err := root.Create("target", 0o644, 0o755, true)
	assert.Nil(t, file)
	require.Error(t, err)
	assert.Less(t, time.Since(start), time.Second)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeNamedPipe)
}
