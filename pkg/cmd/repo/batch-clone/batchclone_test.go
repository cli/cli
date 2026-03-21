package batchclone

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRepoFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "repos.txt")
	err := os.WriteFile(fp, []byte(`
# comment
cli/cli
https://github.com/cli/go-gh.git
git@github.com:cli/shurcooL-graphql.git
`), 0644)
	require.NoError(t, err)

	items, err := parseRepoFile(fp, "workspace")
	require.NoError(t, err)
	require.Len(t, items, 3)

	require.Equal(t, "cli/cli", items[0].Repository)
	require.Equal(t, filepath.Join("workspace", "cli"), items[0].Directory)

	require.Equal(t, "cli/go-gh", items[1].Repository)
	require.Equal(t, filepath.Join("workspace", "go-gh"), items[1].Directory)
}

func TestNormalizeRepoLine_InvalidURL(t *testing.T) {
	_, _, err := normalizeRepoLine("https://github.com/%%%/bad", "")
	require.Error(t, err)
}
