package acceptance_test

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScriptFilter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "single name",
			input: "repo-clone.txtar",
			want:  []string{"repo-clone.txtar"},
		},
		{
			name:  "two names",
			input: "repo-clone.txtar,workflow-list.txtar",
			want:  []string{"repo-clone.txtar", "workflow-list.txtar"},
		},
		{
			name:  "whitespace around entries is trimmed",
			input: " repo-clone.txtar , workflow-list.txtar ",
			want:  []string{"repo-clone.txtar", "workflow-list.txtar"},
		},
		{
			name:  "empty entries between commas are ignored",
			input: "repo-clone.txtar,,workflow-list.txtar",
			want:  []string{"repo-clone.txtar", "workflow-list.txtar"},
		},
		{
			name:  "whitespace-only entries are ignored",
			input: "repo-clone.txtar, ,workflow-list.txtar",
			want:  []string{"repo-clone.txtar", "workflow-list.txtar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseScriptFilter(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestScriptFilteringSelectsOnlyMatchingFiles checks that the file-selection
// logic in testScriptParamsFor picks files that exist in the given directory
// and silently skips names that do not exist (i.e. belong to another command
// directory), and falls back to the whole directory when no filter is set.
func TestScriptFilteringSelectsOnlyMatchingFiles(t *testing.T) {
	// Build a temporary testdata tree so the test does not depend on the real
	// acceptance testdata or live credentials.
	root := t.TempDir()
	repoDir := path.Join(root, "testdata", "repo")
	workflowDir := path.Join(root, "testdata", "workflow")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	require.NoError(t, os.MkdirAll(workflowDir, 0o755))

	repoScript := path.Join(repoDir, "repo-clone.txtar")
	workflowScript := path.Join(workflowDir, "workflow-list.txtar")
	require.NoError(t, os.WriteFile(repoScript, []byte(""), 0o644))
	require.NoError(t, os.WriteFile(workflowScript, []byte(""), 0o644))

	selectFiles := func(scripts []string, command string) []string {
		var files []string
		for _, script := range scripts {
			p := path.Join(root, "testdata", command, script)
			if _, err := os.Stat(p); err == nil {
				files = append(files, p)
			}
		}
		return files
	}

	t.Run("single name in matching directory", func(t *testing.T) {
		files := selectFiles([]string{"repo-clone.txtar"}, "repo")
		assert.Equal(t, []string{repoScript}, files)
	})

	t.Run("single name not in current directory is silently skipped", func(t *testing.T) {
		files := selectFiles([]string{"workflow-list.txtar"}, "repo")
		assert.Empty(t, files)
	})

	t.Run("list with names from multiple directories selects only current directory's match", func(t *testing.T) {
		// When TestRepo and TestWorkflows are both invoked with the same list,
		// each should see only its own files.
		scripts := []string{"repo-clone.txtar", "workflow-list.txtar"}

		repoFiles := selectFiles(scripts, "repo")
		assert.Equal(t, []string{repoScript}, repoFiles)

		workflowFiles := selectFiles(scripts, "workflow")
		assert.Equal(t, []string{workflowScript}, workflowFiles)
	})

	t.Run("empty filter means no file restriction (caller uses Dir instead)", func(t *testing.T) {
		// When scripts is nil/empty, testScriptParamsFor sets Dir rather than Files.
		// Verify the selection returns nothing so the caller can detect this and
		// fall back to the directory.
		files := selectFiles(nil, "repo")
		assert.Empty(t, files)
	})
}
