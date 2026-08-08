package upload

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "foo",
			expected: "foo",
		},
		{
			name:     "foo bar",
			expected: "foo.bar",
		},
		{
			name:     ".foo",
			expected: "default.foo",
		},
		{
			name:     "Foo bar",
			expected: "Foo.bar",
		},
		{
			name:     "Hello, दुनिया",
			expected: "default.Hello",
		},
		{
			name:     "this+has+plusses.jpg",
			expected: "this+has+plusses.jpg",
		},
		{
			name:     "this@has@at@signs.jpg",
			expected: "this@has@at@signs.jpg",
		},
		{
			name:     "façade.exposé",
			expected: "facade.expose",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, shared.SanitizeFileName(tt.name))
		})
	}
}

func Test_NewCmdUpload_rejectsDuplicateAssetFilenames(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	first := filepath.Join(firstDir, "duplicate.zip")
	second := filepath.Join(secondDir, "duplicate.zip")
	require.NoError(t, os.WriteFile(first, nil, 0o644))
	require.NoError(t, os.WriteFile(second, nil, 0o644))

	ios, _, _, _ := iostreams.Test()
	httpClientCalls := 0
	f := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			httpClientCalls++
			return nil, errors.New("unexpected HTTP client call")
		},
	}
	cmd := NewCmdUpload(f, nil)
	cmd.SetArgs([]string{"v1.2.3", first + "#Linux", second + "#macOS"})

	_, err := cmd.ExecuteC()
	require.EqualError(t, err, "duplicate release asset filenames are not supported: duplicate.zip")
	assert.Equal(t, 0, httpClientCalls)
}
