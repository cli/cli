package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputReturnsCommandOutput(t *testing.T) {
	// Given a command that writes to standard output
	cmd, _ := createCommandContext(t, 0, "hello world", "")
	command := Command{cmd}

	// When its output is captured
	out, err := command.Output()

	// Then the output is returned without an error
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(out))
}

func TestOutputReturnsGitError(t *testing.T) {
	// Given a command that fails with Git's not-a-repository diagnostic
	stderr := "fatal: not a git repository (or any of the parent directories): .git"
	cmd, _ := createCommandContext(t, 128, "", stderr)
	command := Command{cmd}

	// When its output is captured
	out, err := command.Output()

	// Then the exit status and diagnostic are preserved in a Git error
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.Equal(t, 128, gitErr.ExitCode)
	assert.Equal(t, stderr, gitErr.Stderr)
	assert.EqualError(t, gitErr, "failed to run git: "+stderr)
	assert.Empty(t, out)
}
