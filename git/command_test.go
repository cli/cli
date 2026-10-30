package git

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPreservesGitErrorDiagnostics(t *testing.T) {
	// Given a command that writes output before failing with a diagnostic on stderr
	const stdout = "partial output\n"
	const stderr = "fatal: test transport failed\n"
	execCmd := createCommand(t, 128, stdout, stderr)
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
	cmd := createCommand(t, 0, "hello world", "")
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
	cmd := createCommand(t, 128, "", stderr)
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

func TestWithRepoDirInsertsRepositoryBeforeGitArguments(t *testing.T) {
	// Given a Git command with a final -C operand after the argument separator
	command := &Command{exec.Command("git", "rev-parse", "--", "-C")}

	// When its repository directory is changed
	WithRepoDir("path/to/repo")(command)

	// Then the repository option is inserted before the Git subcommand without moving the separator
	assert.Equal(t, []string{"git", "-C", "path/to/repo", "rev-parse", "--", "-C"}, command.Args)
}

func TestWithRepoDirReplacesExistingRepository(t *testing.T) {
	// Given a Git command already scoped to a repository
	command := &Command{exec.Command("git", "-C", "old/repo", "status")}

	// When its repository directory is changed
	WithRepoDir("new/repo")(command)

	// Then the existing repository value is replaced without adding another option
	assert.Equal(t, []string{"git", "-C", "new/repo", "status"}, command.Args)
}
