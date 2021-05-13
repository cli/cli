package download

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_extractZip(t *testing.T) {
	tmpDir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })

	zipFile, err := zip.OpenReader("./fixtures/myproject.zip")
	require.NoError(t, err)
	defer zipFile.Close()

	extractPath := filepath.Join(tmpDir, "artifact")
	err = os.MkdirAll(extractPath, 0700)
	require.NoError(t, err)
	require.NoError(t, os.Chdir(extractPath))

	err = extractZip(&zipFile.Reader, ".")
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join("src", "main.go"))
	require.NoError(t, err)
}
