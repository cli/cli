package extension

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/stretchr/testify/require"
)

func TestLsRemoteRequiresAuthenticationConfiguration(t *testing.T) {
	t.Parallel()
	// Given an extension whose account selection cannot be resolved
	want := errors.New("account configuration unavailable")
	client := &gitExecuter{client: &git.Client{
		Authentication: func() (git.Authentication, error) { return git.Authentication{}, want },
	}}

	// When checking the extension's remote version
	_, err := client.LsRemote("origin", "HEAD")

	// Then authentication failure stops the request instead of using ambient credentials
	require.ErrorIs(t, err, want)
}
