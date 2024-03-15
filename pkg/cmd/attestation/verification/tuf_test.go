package verification

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitHubTUFOptions(t *testing.T) {
	os.Setenv("CODESPACES", "true")
	opts := GitHubTUFOptions()

	require.Equal(t, GitHubTUFMirror, opts.RepositoryBaseURL)
	require.NotNil(t, opts.Root)
	require.True(t, opts.DisableLocalCache)
}
