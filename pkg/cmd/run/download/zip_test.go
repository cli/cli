package download

import (
	"archive/zip"
	"os"
	"path/filepath"
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

	err = extractZip(&zipFile.Reader, extractPath)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(extractPath.String(), "src", "main.go"))
	require.NoError(t, err)
}
