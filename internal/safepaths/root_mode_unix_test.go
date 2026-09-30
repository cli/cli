//go:build unix

package safepaths_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileCreationModes(t *testing.T) {
	tests := []struct {
		name         string
		filePerm     os.FileMode
		dirPerm      os.FileMode
		umask        int
		replace      bool
		wantFilePerm os.FileMode
		wantDirPerm  os.FileMode
	}{
		{
			name:         "separate file and directory modes",
			filePerm:     0o600,
			dirPerm:      0o700,
			wantFilePerm: 0o600,
			wantDirPerm:  0o700,
		},
		{
			name:         "creation respects umask",
			filePerm:     0o666,
			dirPerm:      0o777,
			umask:        0o027,
			wantFilePerm: 0o640,
			wantDirPerm:  0o750,
		},
		{
			name:         "replacement preserves existing modes",
			filePerm:     0o666,
			dirPerm:      0o777,
			replace:      true,
			wantFilePerm: 0o640,
			wantDirPerm:  0o750,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Umask is process-wide, so these tests must stay sequential.
			oldUmask := syscall.Umask(tt.umask)
			t.Cleanup(func() { syscall.Umask(oldUmask) })

			for _, operation := range []string{"OpenFile", "WriteFile", "Root.Create", "Root.WriteFile", "Root.CopyFile"} {
				t.Run(operation, func(t *testing.T) {
					rootDir := t.TempDir()
					name := filepath.Join("one", "two", "file.txt")
					path := filepath.Join(rootDir, name)
					if tt.replace {
						require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
						require.NoError(t, os.WriteFile(path, []byte("old"), 0o640))
					}

					root, err := safepaths.OpenRoot(rootDir)
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, root.Close()) })

					var file *os.File
					switch operation {
					case "OpenFile":
						file, err = safepaths.OpenFile(path, tt.filePerm, tt.dirPerm, tt.replace)
					case "WriteFile":
						err = safepaths.WriteFile(path, []byte("new"), tt.filePerm, tt.dirPerm, tt.replace)
					case "Root.Create":
						file, err = root.Create(name, tt.filePerm, tt.dirPerm, tt.replace)
					case "Root.WriteFile":
						err = root.WriteFile(name, []byte("new"), tt.filePerm, tt.dirPerm, tt.replace)
					case "Root.CopyFile":
						err = root.CopyFile(name, strings.NewReader("new"), tt.filePerm, tt.dirPerm, tt.replace)
					}
					require.NoError(t, err)
					if file != nil {
						require.NoError(t, file.Close())
					}

					info, err := os.Stat(path)
					require.NoError(t, err)
					assert.Equal(t, tt.wantFilePerm, info.Mode().Perm())
					for _, dir := range []string{"one", filepath.Join("one", "two")} {
						info, err := os.Stat(filepath.Join(rootDir, dir))
						require.NoError(t, err)
						assert.Equal(t, tt.wantDirPerm, info.Mode().Perm(), dir)
					}
				})
			}
		})
	}
}
