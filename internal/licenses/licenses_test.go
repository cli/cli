package licenses

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestContent_reportOnly(t *testing.T) {
	report := "dep1 (v1.0.0) - MIT - https://example.com\n"
	fsys := fstest.MapFS{
		"third-party/PLACEHOLDER": &fstest.MapFile{Data: []byte("placeholder")},
	}

	actualContent := content(report, fsys, "third-party")

	require.True(t, strings.HasPrefix(actualContent, report), "expected output to start with report")
	require.NotContains(t, actualContent, "PLACEHOLDER")
	require.NotContains(t, actualContent, "====")
}

func TestContent_singleModule(t *testing.T) {
	report := "example.com/mod (v1.0.0) - MIT - https://example.com\n"
	fsys := fstest.MapFS{
		"third-party/example.com/mod/LICENSE": &fstest.MapFile{
			Data: []byte("MIT License\n\nCopyright (c) 2024"),
		},
	}

	actualContent := content(report, fsys, "third-party")

	require.Contains(t, actualContent, filepath.FromSlash("example.com/mod"))
	require.Contains(t, actualContent, "MIT License")
}

func TestContent_multipleModulesSortedAlphabetically(t *testing.T) {
	report := "header\n"
	fsys := fstest.MapFS{
		"third-party/github.com/zzz/pkg/LICENSE": &fstest.MapFile{
			Data: []byte("ZZZ License"),
		},
		"third-party/github.com/aaa/pkg/LICENSE": &fstest.MapFile{
			Data: []byte("AAA License"),
		},
	}

	actualContent := content(report, fsys, "third-party")

	aIdx := strings.Index(actualContent, filepath.FromSlash("github.com/aaa/pkg"))
	zIdx := strings.Index(actualContent, filepath.FromSlash("github.com/zzz/pkg"))
	require.NotEqual(t, -1, aIdx, "expected aaa module in output")
	require.NotEqual(t, -1, zIdx, "expected zzz module in output")
	require.Less(t, aIdx, zIdx, "expected modules to be sorted alphabetically")
}

func TestContent_licenseAndNoticeFiles(t *testing.T) {
	report := "header\n"
	fsys := fstest.MapFS{
		"third-party/example.com/mod/LICENSE": &fstest.MapFile{
			Data: []byte("Apache License 2.0"),
		},
		"third-party/example.com/mod/NOTICE": &fstest.MapFile{
			Data: []byte("Copyright 2024 Example Corp"),
		},
	}

	actualContent := content(report, fsys, "third-party")

	require.Contains(t, actualContent, "Apache License 2.0")
	require.Contains(t, actualContent, "Copyright 2024 Example Corp")
}

func TestContent_emptyThirdPartyDir(t *testing.T) {
	report := "header\n"
	fsys := fstest.MapFS{
		"third-party/empty": &fstest.MapFile{Data: []byte("")},
	}

	actualContent := content(report, fsys, "third-party")

	require.True(t, strings.HasPrefix(actualContent, "header\n"), "expected output to start with report header")
}
