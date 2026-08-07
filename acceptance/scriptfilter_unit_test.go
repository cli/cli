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

// TestSelectScripts exercises the real selectScripts function, verifying that
// it correctly matches files in the command directory, skips names belonging to
// other directories, and reports whether a filter was applied.
func TestSelectScripts(t *testing.T) {
	// Build a temporary testdata tree and change into it so selectScripts can
	// resolve "testdata/<command>/<script>" via os.Stat relative to cwd.
	root := t.TempDir()
	t.Chdir(root)

	repoDir := path.Join(root, "testdata", "repo")
	workflowDir := path.Join(root, "testdata", "workflow")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))
	require.NoError(t, os.MkdirAll(workflowDir, 0o755))

	repoScript := path.Join(repoDir, "repo-clone.txtar")
	workflowScript := path.Join(workflowDir, "workflow-list.txtar")
	require.NoError(t, os.WriteFile(repoScript, []byte(""), 0o644))
	require.NoError(t, os.WriteFile(workflowScript, []byte(""), 0o644))

	tests := []struct {
		name         string
		command      string
		scripts      []string
		wantFiles    []string
		wantFiltered bool
	}{
		{
			name:         "no filter - whole directory should be used",
			command:      "repo",
			scripts:      nil,
			wantFiles:    nil,
			wantFiltered: false,
		},
		{
			name:         "filter matches script in directory",
			command:      "repo",
			scripts:      []string{"repo-clone.txtar"},
			wantFiles:    []string{path.Join("testdata", "repo", "repo-clone.txtar")},
			wantFiltered: true,
		},
		{
			name:         "filter set but no match in this directory",
			command:      "repo",
			scripts:      []string{"workflow-list.txtar"},
			wantFiles:    nil,
			wantFiltered: true,
		},
		{
			name:         "multi-directory filter selects only this directory's script",
			command:      "repo",
			scripts:      []string{"repo-clone.txtar", "workflow-list.txtar"},
			wantFiles:    []string{path.Join("testdata", "repo", "repo-clone.txtar")},
			wantFiltered: true,
		},
		{
			name:         "multi-directory filter selects only workflow directory's script",
			command:      "workflow",
			scripts:      []string{"repo-clone.txtar", "workflow-list.txtar"},
			wantFiles:    []string{path.Join("testdata", "workflow", "workflow-list.txtar")},
			wantFiltered: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, filtered := selectScripts(tt.command, tt.scripts)
			assert.Equal(t, tt.wantFiltered, filtered)
			assert.Equal(t, tt.wantFiles, files)
		})
	}
}
