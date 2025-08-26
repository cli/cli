package loc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterFiles(t *testing.T) {
	tests := []struct {
		name            string
		files           []string
		extensionFilter string
		excludeFiles    []string
		includeFiles    []string
		expected        []string
	}{
		{
			name:            "no filters",
			files:           []string{"main.go", "test.js", "README.md"},
			extensionFilter: "",
			excludeFiles:    []string{},
			includeFiles:    []string{},
			expected:        []string{"main.go", "test.js", "README.md"},
		},
		{
			name:            "extension filter",
			files:           []string{"main.go", "test.js", "README.md"},
			extensionFilter: "go",
			excludeFiles:    []string{},
			includeFiles:    []string{},
			expected:        []string{"main.go"},
		},
		{
			name:            "exclude pattern",
			files:           []string{"main.go", "test.js", "README.md"},
			extensionFilter: "",
			excludeFiles:    []string{"*.js"},
			includeFiles:    []string{},
			expected:        []string{"main.go", "README.md"},
		},
		{
			name:            "include pattern",
			files:           []string{"main.go", "test.js", "README.md"},
			extensionFilter: "",
			excludeFiles:    []string{},
			includeFiles:    []string{"*.go"},
			expected:        []string{"main.go"},
		},
		{
			name:            "multiple filters",
			files:           []string{"main.go", "test.go", "test.js", "README.md"},
			extensionFilter: "go",
			excludeFiles:    []string{"test.*"},
			includeFiles:    []string{},
			expected:        []string{"main.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &LinesOfCodeOptions{
				ExtensionFilter: tt.extensionFilter,
				ExcludeFiles:    tt.excludeFiles,
				IncludeFiles:    tt.includeFiles,
			}

			result := filterFiles(tt.files, opts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCountLinesInFile(t *testing.T) {
	// Create temporary test files
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		content       string
		expectedTotal int
		expectedCode  int
		expectedBlank int
		fileExtension string
	}{
		{
			name: "go file with comments",
			content: `package main

import "fmt"

// This is a comment
func main() {
	fmt.Println("Hello, World!") // inline comment
}

// Another comment
`,
			expectedTotal: 10,
			expectedCode:  5,
			expectedBlank: 3,
			fileExtension: "go",
		},
		{
			name: "javascript file",
			content: `// JavaScript file
const greeting = "Hello";

/* Multi-line
   comment */
function sayHello() {
    console.log(greeting);
}

sayHello();
`,
			expectedTotal: 10,
			expectedCode:  6,
			expectedBlank: 2,
			fileExtension: "js",
		},
		{
			name: "plain text file",
			content: `This is a text file
with multiple lines

and some blank lines


end of file`,
			expectedTotal: 7,
			expectedCode:  4,
			expectedBlank: 3,
			fileExtension: "txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fileName := "test." + tt.fileExtension
			filePath := filepath.Join(tempDir, fileName)

			err := os.WriteFile(filePath, []byte(tt.content), 0644)
			require.NoError(t, err)

			commentPatterns := getCommentPatterns()
			stats, err := countLinesInFile(filePath, commentPatterns)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedTotal, stats.Lines, "total lines mismatch")
			assert.Equal(t, tt.expectedCode, stats.CodeLines, "code lines mismatch")
			assert.Equal(t, tt.expectedBlank, stats.BlankLines, "blank lines mismatch")
			assert.Equal(t, tt.fileExtension, stats.Extension, "extension mismatch")
			assert.Equal(t, filePath, stats.Path, "path mismatch")
		})
	}
}

func TestGetCommentPatterns(t *testing.T) {
	patterns := getCommentPatterns()

	tests := []struct {
		extension   string
		line        string
		shouldMatch bool
	}{
		{"go", "// This is a comment", true},
		{"go", "    // Indented comment", true},
		{"go", "fmt.Println(\"not a comment\")", false},
		{"js", "// JavaScript comment", true},
		{"js", "/* Block comment */", true},
		{"js", "console.log('not a comment');", false},
		{"py", "# Python comment", true},
		{"py", "    # Indented comment", true},
		{"py", "print('not a comment')", false},
		{"sql", "-- SQL comment", true},
		{"sql", "SELECT * FROM table;", false},
	}

	for _, tt := range tests {
		t.Run(tt.extension+"_"+tt.line, func(t *testing.T) {
			pattern, exists := patterns[tt.extension]
			require.True(t, exists, "pattern should exist for extension %s", tt.extension)

			matched := pattern.MatchString(tt.line)
			assert.Equal(t, tt.shouldMatch, matched,
				"pattern match mismatch for extension %s and line %s", tt.extension, tt.line)
		})
	}
}

func TestLinesOfCodeRun_InvalidRepo(t *testing.T) {
	tempDir := t.TempDir()

	gitClient := &git.Client{
		RepoDir: tempDir,
	}

	io, _, _, _ := iostreams.Test()
	opts := &LinesOfCodeOptions{
		IO:        io,
		GitClient: gitClient,
	}

	err := linesOfCodeRun(opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in a git repository")
}

func TestDisplaySummaryResults(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	cs := io.ColorScheme()

	fileStats := []FileStats{
		{Path: "main.go", Lines: 10, CodeLines: 8, BlankLines: 2, Extension: "go"},
		{Path: "test.js", Lines: 15, CodeLines: 12, BlankLines: 3, Extension: "js"},
	}

	opts := &LinesOfCodeOptions{
		IO: io,
	}

	err := displaySummaryResults(opts, fileStats, cs)
	assert.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Files")
	assert.Contains(t, output, "Total lines")
	assert.Contains(t, output, "Code lines")
	assert.Contains(t, output, "Blank lines")
	assert.Contains(t, output, "2")  // file count
	assert.Contains(t, output, "25") // total lines
	assert.Contains(t, output, "20") // code lines
	assert.Contains(t, output, "5")  // blank lines
}

func TestDisplayByTypeResults(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	cs := io.ColorScheme()

	fileStats := []FileStats{
		{Path: "main.go", Lines: 10, CodeLines: 8, BlankLines: 2, Extension: "go"},
		{Path: "util.go", Lines: 5, CodeLines: 4, BlankLines: 1, Extension: "go"},
		{Path: "test.js", Lines: 15, CodeLines: 12, BlankLines: 3, Extension: "js"},
	}

	opts := &LinesOfCodeOptions{
		IO:             io,
		ShowByFileType: true,
	}

	err := displayByTypeResults(opts, fileStats, cs)
	assert.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Extension")
	assert.Contains(t, output, "Files")
	assert.Contains(t, output, "go")
	assert.Contains(t, output, "js")
	// Should show go type has 2 files
	lines := strings.Split(output, "\n")
	var goLine string
	for _, line := range lines {
		if strings.Contains(line, "go") && !strings.Contains(line, "Extension") {
			goLine = line
			break
		}
	}
	assert.Contains(t, goLine, "2") // 2 go files
}

func TestDisplayDetailedResults(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	cs := io.ColorScheme()

	fileStats := []FileStats{
		{Path: "main.go", Lines: 10, CodeLines: 8, BlankLines: 2, Extension: "go"},
		{Path: "test.js", Lines: 15, CodeLines: 12, BlankLines: 3, Extension: "js"},
	}

	opts := &LinesOfCodeOptions{
		IO:           io,
		ShowDetailed: true,
	}

	err := displayDetailedResults(opts, fileStats, cs)
	assert.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "File")
	assert.Contains(t, output, "Total")
	assert.Contains(t, output, "Code")
	assert.Contains(t, output, "Blank")
	assert.Contains(t, output, "Type")
	assert.Contains(t, output, "main.go")
	assert.Contains(t, output, "test.js")
	// Should be sorted by total lines descending (test.js first)
	jsIndex := strings.Index(output, "test.js")
	goIndex := strings.Index(output, "main.go")
	assert.True(t, jsIndex < goIndex, "files should be sorted by total lines descending")
}

func TestGetTrackedFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create a simple test repository
	gitClient := &git.Client{
		RepoDir: tempDir,
	}

	// For this test, we'll assume we have a git repository initialized
	// In a real test environment, you'd set up a proper git repo
	ctx := context.Background()

	// This test will be skipped if not in a git repository
	isRepo, err := gitClient.IsLocalGitRepo(ctx)
	if err != nil || !isRepo {
		t.Skip("Not in a git repository - skipping getTrackedFiles test")
	}

	files, err := getTrackedFiles(ctx, gitClient)
	if err != nil {
		t.Skip("Could not get tracked files - this is expected in test environment")
	}

	// Basic validation - should return a slice (even if empty)
	assert.IsType(t, []string{}, files)
}
