package verification

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestGitHubTUFOptions(t *testing.T) {
	os.Setenv("CODESPACES", "true")
	opts := GitHubTUFOptions()

	require.Equal(t, GitHubTUFMirror, opts.RepositoryBaseURL)
	require.NotNil(t, opts.Root)
	require.True(t, opts.DisableLocalCache)
	require.Equal(t, filepath.Join(config.CacheDir(), ".sigstore", "root"), opts.CachePath)
}
