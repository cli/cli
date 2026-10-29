package git

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPreservesGitErrorDiagnostics(t *testing.T) {
	// Given a command that writes output before failing with a diagnostic on stderr
	const stdout = "partial output\n"
	const stderr = "fatal: test transport failed\n"
	execCmd, _ := createCommandContext(t, 128, stdout, stderr)
	stdoutBuffer := &bytes.Buffer{}
	execCmd.Stdout = stdoutBuffer
	cmd := Command{execCmd}

	// When the command is run
	err := cmd.Run()

	// Then stdout is written and the Git error preserves the diagnostic and exit code
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.Equal(t, stdout, stdoutBuffer.String())
	assert.Equal(t, 128, gitErr.ExitCode)
	assert.Equal(t, stderr, gitErr.Stderr)
	require.EqualError(t, err, "failed to run git: fatal: test transport failed\n")
}

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
	require.EqualError(t, gitErr, "failed to run git: "+stderr)
	assert.Empty(t, out)
}
