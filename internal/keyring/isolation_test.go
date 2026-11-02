//go:build ghdebug

package keyring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisabledKeyringCannotReadReplaceOrDeleteExistingCredentials(t *testing.T) {
	// Given a fake OS keyring holding the operator's credential
	MockInit()
	t.Setenv("GH_INTERNAL_DISABLE_KEYRING", "false")
	require.NoError(t, Set("gh:example.test", "alice", "operator-credential"))
	t.Setenv("GH_INTERNAL_DISABLE_KEYRING", "true")

	// When an isolated scenario tries every keyring operation
	secret, err := Get("gh:example.test", "alice")
	require.ErrorIs(t, err, ErrNotFound)
	assert.Empty(t, secret)
	require.Error(t, Set("gh:example.test", "alice", "scenario-credential"))
	require.ErrorIs(t, Delete("gh:example.test", "alice"), ErrNotFound)

	// Then normal keyring access afterward still sees the original credential
	t.Setenv("GH_INTERNAL_DISABLE_KEYRING", "false")
	secret, err = Get("gh:example.test", "alice")
	require.NoError(t, err)
	assert.Equal(t, "operator-credential", secret)
}
