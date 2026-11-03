package identity

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExplainUnboundRepositoryUsesRequestedDirectory(t *testing.T) {
	// Given a repository with ordinary Git attribution and no identity bindings
	store := isolatedStore(t)
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "ordinary@example.invalid")
	canonical, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)

	// When explaining that repository rather than the process working directory
	explanation, err := store.Explain("github.com", "", repo)

	// Then context and observed Git settings still describe the requested repository
	require.NoError(t, err)
	assert.Equal(t, canonical, explanation.Directory)
	assert.Equal(t, canonical, explanation.Repository)
	assert.Equal(t, "ordinary@example.invalid", explanation.GitUserEmail.Value)
	assert.Nil(t, explanation.Identity)
}
