package verification

import (
	"os"
	"path/filepath"
	"testing"

	o "github.com/cli/cli/v2/pkg/option"
	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestGitHubTUFOptionsNoMetadataDir(t *testing.T) {
	os.Setenv("CODESPACES", "true")
	opts := GitHubTUFOptions(o.None[string]())

	require.Equal(t, GitHubTUFMirror, opts.RepositoryBaseURL)
	require.NotNil(t, opts.Root)
	require.True(t, opts.DisableLocalCache)
	require.Equal(t, filepath.Join(config.CacheDir(), ".sigstore", "root"), opts.CachePath)
}

func TestGitHubTUFOptionsWithMetadataDir(t *testing.T) {
	opts := GitHubTUFOptions(o.Some("anything"))
	require.Equal(t, "anything", opts.CachePath)
}
