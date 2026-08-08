package flock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryLock(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string // returns lock path
		wantErr error
		verify  func(t *testing.T, f *os.File)
	}{
		{
			name: "acquires lock and returns writable file handle",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "test.lock")
			},
			verify: func(t *testing.T, f *os.File) {
				t.Helper()
				_, err := f.WriteString("hello")
				require.NoError(t, err)
				_, err = f.Seek(0, 0)
				require.NoError(t, err)
				buf := make([]byte, 5)
				n, err := f.Read(buf)
				assert.NoError(t, err)
				assert.Equal(t, "hello", string(buf[:n]))
			},
		},
		{
			name: "creates lock file if it does not exist",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "subdir")
				require.NoError(t, os.MkdirAll(dir, 0o755))
				return filepath.Join(dir, "new.lock")
			},
			verify: func(t *testing.T, f *os.File) {
				t.Helper()
				_, err := os.Stat(f.Name())
				assert.NoError(t, err)
			},
		},
		{
			name: "second lock on same path returns ErrLocked",
			setup: func(t *testing.T) string {
				lockPath := filepath.Join(t.TempDir(), "contended.lock")
				_, unlock, err := flock.TryLock(lockPath)
				require.NoError(t, err)
				t.Cleanup(unlock)
				return lockPath
			},
			wantErr: flock.ErrLocked,
		},
		{
			name: "lock succeeds after unlock",
			setup: func(t *testing.T) string {
				lockPath := filepath.Join(t.TempDir(), "reuse.lock")
				_, unlock, err := flock.TryLock(lockPath)
				require.NoError(t, err)
				unlock()
				return lockPath
			},
		},
		{
			name: "fails on non-existent directory",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "no", "such", "dir", "test.lock")
			},
			wantErr: os.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lockPath := tt.setup(t)

			f, unlock, err := flock.TryLock(lockPath)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, f)
			defer unlock()

			if tt.verify != nil {
				tt.verify(t, f)
			}
		})
	}
}
