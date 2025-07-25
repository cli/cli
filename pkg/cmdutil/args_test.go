package cmdutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMinimumArgs(t *testing.T) {
	tests := []struct {
		N    int
		Args []string
	}{
		{
			N:    1,
			Args: []string{"v1.2.3"},
		},
		{
			N:    2,
			Args: []string{"v1.2.3", "cli/cli"},
		},
	}

	for _, test := range tests {
		if got := MinimumArgs(test.N, "")(nil, test.Args); got != nil {
			t.Errorf("Got: %v, Want: (nil)", got)
		}
	}
}

func TestMinimumNs_with_error(t *testing.T) {
	tests := []struct {
		N             int
		CustomMessage string
		WantMessage   string
	}{
		{
			N:             1,
			CustomMessage: "A custom msg",
			WantMessage:   "A custom msg",
		},
		{
			N:             1,
			CustomMessage: "",
			WantMessage:   "requires at least 1 arg(s), only received 0",
		},
	}

	for _, test := range tests {
		if got := MinimumArgs(test.N, test.CustomMessage)(nil, nil); got.Error() != test.WantMessage {
			t.Errorf("Got: %v, Want: %v", got, test.WantMessage)
		}
	}
}

func TestPartition(t *testing.T) {
	tests := []struct {
		name            string
		slice           []any
		predicate       func(any) bool
		wantMatching    []any
		wantNonMatching []any
	}{
		{
			name:  "When the slice is empty, it returns two empty slices",
			slice: []any{},
			predicate: func(any) bool {
				return true
			},
			wantMatching:    []any{},
			wantNonMatching: []any{},
		},
		{
			name: "when the slice has one element that satisfies the predicate, it returns a slice with that element and an empty slice",
			slice: []any{
				"foo",
			},
			predicate: func(any) bool {
				return true
			},
			wantMatching:    []any{"foo"},
			wantNonMatching: []any{},
		},
		{
			name: "when the slice has one element that does not satisfy the predicate, it returns an empty slice and a slice with that element",
			slice: []any{
				"foo",
			},
			predicate: func(any) bool {
				return false
			},
			wantMatching:    []any{},
			wantNonMatching: []any{"foo"},
		},
		{
			name: "when the slice has multiple elements, it returns a slice with the elements that satisfy the predicate and a slice with the elements that do not satisfy the predicate",
			slice: []any{
				"foo",
				"bar",
				"baz",
			},
			predicate: func(s any) bool {
				return s.(string) != "foo"
			},
			wantMatching:    []any{"bar", "baz"},
			wantNonMatching: []any{"foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMatching, gotNonMatching := Partition(tt.slice, tt.predicate)
			assert.ElementsMatch(t, tt.wantMatching, gotMatching)
			assert.ElementsMatch(t, tt.wantNonMatching, gotNonMatching)
		})
	}
}

func TestGlobPaths(t *testing.T) {
	tests := []struct {
		name     string
		os       string
		patterns []string
		wantOut  []string
		wantErr  error
	}{
		{
			name:     "When no patterns are passed, return an empty slice",
			patterns: []string{},
			wantOut:  []string{},
			wantErr:  nil,
		},
		{
			name:     "When no files match, it returns an empty expansions array with error",
			patterns: []string{"foo"},
			wantOut:  []string{},
			wantErr:  errors.New("no matches found for `foo`"),
		},
		{
			name: "When a single pattern, '*.txt' is passed with one match, it returns that match",
			patterns: []string{
				"*.txt",
			},
			wantOut: []string{
				"rootFile.txt",
			},
			wantErr: nil,
		},
		{
			name: "When a single pattern, '*/*.txt' is passed with multiple matches, it returns those matches",
			patterns: []string{
				"*/*.txt",
			},
			wantOut: []string{
				filepath.Join("subDir1", "subDir1_file.txt"),
				filepath.Join("subDir2", "subDir2_file.txt"),
			},
			wantErr: nil,
		},
		{
			name: "When multiple patterns, '*/*.txt' and '*/*.go', are passed with multiple matches, it returns those matches",
			patterns: []string{
				"*/*.txt",
				"*/*.go",
			},
			wantOut: []string{
				filepath.Join("subDir1", "subDir1_file.txt"),
				filepath.Join("subDir2", "subDir2_file.txt"),
				filepath.Join("subDir2", "subDir2_file.go"),
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupFn := createTestDir(t)
			defer cleanupFn()

			got, err := GlobPaths(tt.patterns)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOut, got)
		})
	}
}

// Creates a temporary directory with the structure below. Returns
// a cleanup function that will remove the directory and all of its
// contents. The cleanup function should be wrapped in a defer statement.
//
//	| root
//	|-- rootFile.txt
//	|-- subDir1
//	|	|-- subDir1_file.txt
//	|
//	|-- subDir2
//		|-- subDir2_file.go
//		|-- subDir2_file.txt
func createTestDir(t *testing.T) (cleanupFn func()) {
	t.Helper()
	// Make Directories
	rootDir := t.TempDir()

	// Move workspace to temporary directory
	t.Chdir(rootDir)

	// Make subdirectories
	err := os.Mkdir(filepath.Join(rootDir, "subDir1"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(filepath.Join(rootDir, "subDir2"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Make Files
	err = os.WriteFile(filepath.Join(rootDir, "rootFile.txt"), []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(rootDir, "subDir1", "subDir1_file.txt"), []byte(""), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(rootDir, "subDir2", "subDir2_file.go"), []byte(""), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(rootDir, "subDir2", "subDir2_file.txt"), []byte(""), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cleanupFn = func() {
		os.RemoveAll(rootDir)
	}
	return cleanupFn
}
