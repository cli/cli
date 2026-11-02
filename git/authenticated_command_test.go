package git

import (
	"context"
	"errors"
	"testing"

	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedCommandResolvesAccountAfterClientCopy(t *testing.T) {
	t.Parallel()
	// Given a client copied before the invocation selects an account
	account := ""
	client := &Client{
		GhPath: "/tools/gh", GitPath: "/tools/git",
		Authentication: func() (Authentication, error) {
			return Authentication{
				Environment:  []string{"GH_IDENTITY_HOST=github.com", "GH_IDENTITY_ACCOUNT=" + account},
				UseSSHHelper: true,
			}, nil
		},
	}
	copy := client.Copy()
	account = "bob"

	// When constructing an authenticated command through the copy
	cmd, err := copy.AuthenticatedCommand(context.Background(), AllMatchingCredentialsPattern, "fetch")

	// Then one helper chain and both transport overrides use the current selection
	require.NoError(t, err)
	assert.Equal(t, []string{"/tools/git", "-c", "credential.helper=", "-c",
		`credential.helper=!"/tools/gh" auth git-credential`, "fetch"}, cmd.Args)
	assert.Contains(t, cmd.Env, "GH_IDENTITY_HOST=github.com")
	assert.Contains(t, cmd.Env, "GH_IDENTITY_ACCOUNT=bob")
	assert.Contains(t, cmd.Env, "GIT_SSH_COMMAND="+shellquote.Join("/tools/gh", "auth", "git-ssh"))
	assert.Contains(t, cmd.Env, "GIT_SSH_VARIANT=ssh")
}

func TestLocalCommandDoesNotResolveAuthentication(t *testing.T) {
	t.Parallel()
	// Given an authentication provider that cannot load configuration
	client := &Client{
		GitPath: "/tools/git",
		Authentication: func() (Authentication, error) {
			return Authentication{}, errors.New("authentication unavailable")
		},
	}

	// When constructing a local Git command
	cmd, err := client.Command(context.Background(), "status")

	// Then local work does not depend on authentication or acquire overrides
	require.NoError(t, err)
	assert.Equal(t, []string{"/tools/git", "status"}, cmd.Args)
	assert.Nil(t, cmd.Env)
}

func TestAuthenticatedCommandReportsAccountResolutionFailure(t *testing.T) {
	t.Parallel()
	// Given authentication configuration that cannot be loaded
	want := errors.New("authentication unavailable")
	client := &Client{
		GitPath:        "/tools/git",
		Authentication: func() (Authentication, error) { return Authentication{}, want },
	}

	// When constructing an authenticated Git command
	_, err := client.AuthenticatedCommand(context.Background(), AllMatchingCredentialsPattern, "fetch")

	// Then the failure is returned instead of running with another account
	require.ErrorIs(t, err, want)
}
