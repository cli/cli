package batchclone

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeRepoLine(t *testing.T) {
	repo, dest, err := normalizeRepoLine("cli/cli", "workspace")
	require.NoError(t, err)
	require.Equal(t, "cli/cli", repo)
	require.Equal(t, filepath.Join("workspace", "cli"), dest)
}

func TestNormalizeRepoLine_HTTPS(t *testing.T) {
	repo, dest, err := normalizeRepoLine("https://github.com/cli/cli.git", "")
	require.NoError(t, err)
	require.Equal(t, "cli/cli", repo)
	require.Equal(t, "cli", dest)
}

func TestNormalizeRepoLine_SSH(t *testing.T) {
	repo, dest, err := normalizeRepoLine("git@github.com:cli/cli.git", "")
	require.NoError(t, err)
	require.Equal(t, "cli/cli", repo)
	require.Equal(t, "cli", dest)
}

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

	entries, err := parseRepoFile(fp, "workspace")
	require.NoError(t, err)
	require.Len(t, entries, 3)
}
