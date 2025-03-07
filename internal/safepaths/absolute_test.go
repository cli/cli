package safepaths_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/stretchr/testify/require"
)

func TestParseAbsolutePath(t *testing.T) {
	t.Parallel()

	absolutePath, err := safepaths.ParseAbsolute("/base")
	require.NoError(t, err)

	require.Equal(t, filepath.Join(rootDir(), "base"), absolutePath.String())
}

func TestAbsoluteEmptyPathStringPanic(t *testing.T) {
	t.Parallel()

	absolutePath := safepaths.Absolute{}
	require.Panics(t, func() {
		_ = absolutePath.String()
	})
}

func TestJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		base                   safepaths.Absolute
		elems                  []string
		want                   safepaths.Absolute
		wantPathTraversalError bool
	}{
		{
			name:  "child of base",
			base:  mustParseAbsolute("/base"),
			elems: []string{"child"},
			want:  mustParseAbsolute("/base/child"),
		},
		{
			name:  "grandchild of base",
			base:  mustParseAbsolute("/base"),
			elems: []string{"child", "grandchild"},
			want:  mustParseAbsolute("/base/child/grandchild"),
		},
		{
			name:                   "relative parent of base",
			base:                   mustParseAbsolute("/base"),
			elems:                  []string{".."},
			wantPathTraversalError: true,
		},
		{
			name:                   "relative grandparent of base",
			base:                   mustParseAbsolute("/base"),
			elems:                  []string{"..", ".."},
			wantPathTraversalError: true,
		},
		{
			name:  "relative current dir",
			base:  mustParseAbsolute("/base"),
			elems: []string{"."},
			want:  mustParseAbsolute("/base"),
		},
		{
			name:  "subpath via relative parent",
			base:  mustParseAbsolute("/child"),
			elems: []string{"..", "child"},
			want:  mustParseAbsolute("/child"),
		},
		{
			name:  "empty string",
			base:  mustParseAbsolute("/base"),
			elems: []string{""},
			want:  mustParseAbsolute("/base"),
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			joinedPath, err := tt.base.Join(tt.elems...)
			if tt.wantPathTraversalError {
				var pathTraversalError safepaths.PathTraversalError
				require.ErrorAs(t, err, &pathTraversalError)
				require.Equal(t, tt.base, pathTraversalError.Base)
				require.Equal(t, tt.elems, pathTraversalError.Elems)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, joinedPath)
		})
	}
}

func TestPathTraversalErrorMessage(t *testing.T) {
	t.Parallel()

	pathTraversalError := safepaths.PathTraversalError{
		Base:  mustParseAbsolute("/base"),
		Elems: []string{".."},
	}
	expectedMsg := fmt.Sprintf("joining %s and %s would be a traversal", filepath.Join(rootDir(), "base"), "..")
	require.EqualError(t, pathTraversalError, expectedMsg)
}

func mustParseAbsolute(s string) safepaths.Absolute {
	t, err := safepaths.ParseAbsolute(s)
	if err != nil {
		panic(err)
	}
	return t
}

func rootDir() string {
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	// For Windows, extract the volume and add back the root
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(cwd)
		return volume + "\\"
	}

	// For Unix-based systems, the root is always "/"
	return "/"
}
