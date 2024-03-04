package verification

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitHubTUFOptions(t *testing.T) {
	os.Setenv("CODESPACES", "true")
	opts, err := GitHubTUFOptions()
	assert.NoError(t, err)

	assert.Equal(t, GitHubTUFMirror, opts.RepositoryBaseURL)
	assert.NotNil(t, opts.Root)
	assert.True(t, opts.DisableLocalCache)
}
