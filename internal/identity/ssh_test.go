package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHUsesRepositoryIdentityWithoutChangingRemote(t *testing.T) {
	// Given an repository identity with an SSH alias
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "github-work"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh"))
	original := []string{"-o", "SendEnv=GIT_PROTOCOL", "git@github.com", "git-upload-pack 'company/project.git'"}

	// When independently invoked Git dispatches its SSH connection
	got, err := s.SSHArguments(original, dir)

	// Then only the invocation's SSH host changes, not the canonical URL or input
	require.NoError(t, err)
	assert.Equal(t, []string{"-o", "SendEnv=GIT_PROTOCOL", "git@github-work", "git-upload-pack 'company/project.git'"}, got)
	assert.Equal(t, "git@github.com", original[2])
}

func TestMissingSSHRouteFailsRatherThanUsingDefaultKey(t *testing.T) {
	// Given an HTTPS-capable identity without an SSH route
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh"))

	// When Git attempts an SSH operation in that repository
	_, err := s.SSHArguments([]string{"git@github.com", "git-upload-pack 'company/repo.git'"}, dir)

	// Then routing fails explicitly instead of trying an unrelated default key
	require.ErrorContains(t, err, "has no SSH route")
}

func TestSSHPreservesCertificateAuthorityUser(t *testing.T) {
	// Given an repository identity and GitHub's organization-specific SSH certificate user
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	require.NoError(t, s.Save(Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "github-work"}, false))
	require.NoError(t, s.Bind(Binding{Identity: "work", Directory: dir}, "/path/to/gh"))

	// When Git connects using the server-supplied SSH URL
	got, err := s.SSHArguments([]string{"org-1234@github.com", "git-upload-pack 'company/project.git'"}, dir)

	// Then identity routing changes the host without changing the SSH certificate user
	require.NoError(t, err)
	assert.Equal(t, "org-1234@github-work", got[0])
}
