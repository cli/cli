package cmdutil

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stdinReader is standard input that records whether it was read.
type stdinReader struct {
	io.Reader
	read bool
}

func (r *stdinReader) Read(p []byte) (int, error) {
	r.read = true
	return r.Reader.Read(p)
}

func (r *stdinReader) Close() error { return nil }

func TestReadMarkdownBodyFile(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		path        string
		stdin       string
		wantContent string
		wantDir     string
		wantErrIs   error
	}{
		{
			name:        "a file in the working directory",
			files:       map[string]string{"body.md": "![login](./login.png)\n"},
			path:        "body.md",
			wantContent: "![login](./login.png)\n",
			wantDir:     ".",
		},
		{
			name:        "a file in another directory",
			files:       map[string]string{"docs/body.md": "![login](./login.png)\n"},
			path:        "docs/body.md",
			wantContent: "![login](./login.png)\n",
			wantDir:     "docs",
		},
		{
			name:        "standard input has no directory",
			path:        "-",
			stdin:       "![login](./login.png)\n",
			wantContent: "![login](./login.png)\n",
			wantDir:     "",
		},
		{
			name:        "standard input is only read for -",
			files:       map[string]string{"body.md": "From the file\n"},
			path:        "body.md",
			stdin:       "From standard input\n",
			wantContent: "From the file\n",
			wantDir:     ".",
		},
		{
			name:      "a missing file",
			path:      "docs/missing.md",
			wantErrIs: fs.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for path, content := range tt.files {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
				require.NoError(t, os.WriteFile(path, []byte(content), 0644))
			}
			stdin := &stdinReader{Reader: strings.NewReader(tt.stdin)}

			content, dir, err := ReadMarkdownBodyFile(tt.path, stdin)

			assert.Equal(t, tt.path == "-", stdin.read, "standard input is read only for -")
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Empty(t, content)
				assert.Empty(t, dir)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, content)
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}
