package zip

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_extractZip(t *testing.T) {
	tests := []struct {
		name        string
		directories []string
		entries     []string
		setup       func(t *testing.T, extractPath, outside string)
		wantExtract []string
		wantDirs    []string
		wantErr     string
	}{
		{
			name:        "extracts nested file",
			entries:     []string{"src/main.go"},
			wantExtract: []string{"src/main.go"},
		},
		{
			name:    "skips lexical escape",
			entries: []string{"../outside.txt"},
		},
		{
			name: "skips escapes among safe entries",
			entries: []string{
				"../outside-before.txt",
				"safe-one.txt",
				"../../outside-between.txt",
				"nested/safe-two.txt",
			},
			wantExtract: []string{"safe-one.txt", "nested/safe-two.txt"},
		},
		{
			name:        "extracts explicit directory",
			directories: []string{"nested/"},
			wantDirs:    []string{"nested"},
		},
		{
			name:        "explicit directory rejects planted link",
			directories: []string{"nested/"},
			setup: func(t *testing.T, extractPath, outside string) {
				t.Helper()
				require.NoError(t, os.Symlink(outside, filepath.Join(extractPath, "nested")))
			},
			wantErr: "symbolic link",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			for _, name := range tt.directories {
				header := &zip.FileHeader{Name: name}
				header.SetMode(os.ModeDir | 0o755)
				_, err := writer.CreateHeader(header)
				require.NoError(t, err)
			}
			for _, name := range tt.entries {
				file, err := writer.Create(name)
				require.NoError(t, err)
				_, err = file.Write([]byte("content"))
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())

			reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
			require.NoError(t, err)
			extractPath := t.TempDir()
			outside := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, extractPath, outside)
			}
			root, err := safepaths.OpenRoot(extractPath)
			require.NoError(t, err)
			defer root.Close()

			err = ExtractZip(reader, root)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, mustReadDir(t, outside))
				return
			}
			require.NoError(t, err)
			for _, name := range tt.wantExtract {
				content, err := os.ReadFile(filepath.Join(extractPath, filepath.FromSlash(name)))
				require.NoError(t, err)
				assert.Equal(t, "content", string(content))
			}
			for _, name := range tt.wantDirs {
				require.DirExists(t, filepath.Join(extractPath, filepath.FromSlash(name)))
			}
			assert.NoFileExists(t, filepath.Join(extractPath, "..", "outside-before.txt"))
			assert.NoFileExists(t, filepath.Join(extractPath, "..", "..", "outside-between.txt"))
		})
	}
}

func mustReadDir(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(path)
	require.NoError(t, err)
	return entries
}
