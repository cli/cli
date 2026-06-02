package zip

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/stretchr/testify/require"
)

func Test_extractZip(t *testing.T) {
	tmpDir := t.TempDir()
	extractPath, err := safepaths.ParseAbsolute(filepath.Join(tmpDir, "artifact"))
	require.NoError(t, err)

	zipFile, err := zip.OpenReader("./fixtures/myproject.zip")
	require.NoError(t, err)
	defer zipFile.Close()

	err = ExtractZip(&zipFile.Reader, extractPath)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(extractPath.String(), "src", "main.go"))
	require.NoError(t, err)
}

// makeZipReader builds an in-memory zip archive from name->content pairs.
func makeZipReader(t *testing.T, files map[string]string) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return zr
}

func Test_extractZip_withinTotalSizeBudget(t *testing.T) {
	extractPath, err := safepaths.ParseAbsolute(filepath.Join(t.TempDir(), "out"))
	require.NoError(t, err)

	zr := makeZipReader(t, map[string]string{"a.txt": "hello"})

	require.NoError(t, ExtractZipWithLimit(zr, extractPath, 1<<20))

	got, err := os.ReadFile(filepath.Join(extractPath.String(), "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
}

func Test_extractZip_exceedsTotalSizeBudget(t *testing.T) {
	extractPath, err := safepaths.ParseAbsolute(filepath.Join(t.TempDir(), "out"))
	require.NoError(t, err)

	// Two highly-compressible entries totalling 1200 uncompressed bytes; a
	// 1000-byte budget must reject the archive (a decompression-bomb guard that
	// counts bytes actually written, not the compressed footprint).
	zr := makeZipReader(t, map[string]string{
		"a.txt": strings.Repeat("A", 600),
		"b.txt": strings.Repeat("B", 600),
	})

	err = ExtractZipWithLimit(zr, extractPath, 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum allowed uncompressed")
}
