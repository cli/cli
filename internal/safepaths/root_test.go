package safepaths_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCreate(t *testing.T) {
	var preservedMode os.FileMode

	tests := []struct {
		name    string
		path    string
		replace bool
		setup   func(t *testing.T, root, outside string)
		wantErr string
		verify  func(t *testing.T, root, outside string)
	}{
		{
			name: "creates nested file",
			path: "nested/file.txt",
			verify: func(t *testing.T, root, _ string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(root, "nested", "file.txt"))
				require.NoError(t, err)
				assert.Equal(t, "new", string(content))
			},
		},
		{
			name:    "rejects root directory as file",
			path:    ".",
			wantErr: "root directory",
		},
		{
			name:    "rejects lexical escape",
			path:    "../file.txt",
			wantErr: "not a local child path",
		},
		{
			name:    "rejects trailing separator",
			path:    "nested" + string(os.PathSeparator),
			wantErr: "ends in a separator",
			verify: func(t *testing.T, root, _ string) {
				t.Helper()
				_, err := os.Lstat(filepath.Join(root, "nested"))
				require.True(t, os.IsNotExist(err))
				assert.NoFileExists(t, filepath.Join(root, "nested", "nested"))
			},
		},
		{
			name: "rejects ancestor symlink",
			path: "nested/file.txt",
			setup: func(t *testing.T, root, outside string) {
				t.Helper()
				require.NoError(t, os.Symlink(outside, filepath.Join(root, "nested")))
			},
			wantErr: "symbolic link",
			verify: func(t *testing.T, _, outside string) {
				t.Helper()
				assert.NoFileExists(t, filepath.Join(outside, "file.txt"))
			},
		},
		{
			name: "no clobber rejects dangling final symlink",
			path: "file.txt",
			setup: func(t *testing.T, root, outside string) {
				t.Helper()
				require.NoError(t, os.Symlink(filepath.Join(outside, "missing.txt"), filepath.Join(root, "file.txt")))
			},
			wantErr: "file exists",
			verify: func(t *testing.T, _, outside string) {
				t.Helper()
				assert.NoFileExists(t, filepath.Join(outside, "missing.txt"))
			},
		},
		{
			name:    "clobber replaces final symlink",
			path:    "file.txt",
			replace: true,
			setup: func(t *testing.T, root, outside string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(root, "target.txt"), []byte("old"), 0o644))
				require.NoError(t, os.Symlink("target.txt", filepath.Join(root, "file.txt")))
			},
			verify: func(t *testing.T, root, outside string) {
				t.Helper()
				info, err := os.Lstat(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.Zero(t, info.Mode()&os.ModeSymlink)
				content, err := os.ReadFile(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.Equal(t, "new", string(content))
				content, err = os.ReadFile(filepath.Join(root, "target.txt"))
				require.NoError(t, err)
				assert.Equal(t, "old", string(content))
			},
		},
		{
			name:    "clobber refuses symlink to directory",
			path:    "file.txt",
			replace: true,
			setup: func(t *testing.T, root, outside string) {
				t.Helper()
				require.NoError(t, os.Mkdir(filepath.Join(root, "target"), 0o755))
				require.NoError(t, os.Symlink("target", filepath.Join(root, "file.txt")))
			},
			wantErr: "is a directory",
			verify: func(t *testing.T, root, _ string) {
				t.Helper()
				info, err := os.Lstat(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
			},
		},
		{
			name:    "clobber refuses redirect outside root",
			path:    "file.txt",
			replace: true,
			setup: func(t *testing.T, root, outside string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(outside, "target.txt"), []byte("old"), 0o644))
				require.NoError(t, os.Symlink(filepath.Join(outside, "target.txt"), filepath.Join(root, "file.txt")))
			},
			wantErr: "could not safely classify redirect target",
			verify: func(t *testing.T, root, outside string) {
				t.Helper()
				info, err := os.Lstat(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.NotZero(t, info.Mode()&os.ModeSymlink)
				content, err := os.ReadFile(filepath.Join(outside, "target.txt"))
				require.NoError(t, err)
				assert.Equal(t, "old", string(content))
			},
		},
		{
			name:    "clobber replaces dangling redirect",
			path:    "file.txt",
			replace: true,
			setup: func(t *testing.T, root, _ string) {
				t.Helper()
				require.NoError(t, os.Symlink("missing.txt", filepath.Join(root, "file.txt")))
			},
			verify: func(t *testing.T, root, _ string) {
				t.Helper()
				info, err := os.Lstat(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.Zero(t, info.Mode()&os.ModeSymlink)
				assert.NoFileExists(t, filepath.Join(root, "missing.txt"))
			},
		},
		{
			name:    "clobber preserves regular file mode",
			path:    "file.txt",
			replace: true,
			setup: func(t *testing.T, root, _ string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("old"), 0o600))
				info, err := os.Stat(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				preservedMode = info.Mode().Perm()
			},
			verify: func(t *testing.T, root, _ string) {
				t.Helper()
				info, err := os.Stat(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.Equal(t, preservedMode, info.Mode().Perm())
				content, err := os.ReadFile(filepath.Join(root, "file.txt"))
				require.NoError(t, err)
				assert.Equal(t, "new", string(content))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootDir := t.TempDir()
			outside := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, rootDir, outside)
			}

			root, err := safepaths.OpenRoot(rootDir)
			require.NoError(t, err)
			defer root.Close()

			err = root.WriteFile(tt.path, []byte("new"), 0o644, 0o755, tt.replace)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.verify != nil {
				tt.verify(t, rootDir, outside)
			}
		})
	}
}

func TestValidateChild(t *testing.T) {
	root, err := safepaths.OpenRoot(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, root.Close()) })

	validators := []struct {
		name     string
		validate func(string) error
	}{
		{name: "ValidateChild", validate: safepaths.ValidateChild},
		{name: "Root.Validate", validate: root.Validate},
	}
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "empty path cleans to root", path: ""},
		{name: "root", path: "."},
		{name: "local child", path: "nested/file.txt"},
		{name: "contained parent component", path: "nested/../file.txt"},
		{name: "trailing separator", path: "nested/"},
		{name: "parent escape", path: "../file.txt", wantErr: true},
		{name: "nested parent escape", path: "nested/../../file.txt", wantErr: true},
		{name: "absolute", path: filepath.Join(string(os.PathSeparator), "file.txt"), wantErr: true},
		{name: "backslash child", path: `nested\file.txt`},
		{name: "backslash parent escape", path: `..\..\file.txt`, wantErr: runtime.GOOS == "windows"},
		{name: "mixed separator escape", path: `nested/..\..\file.txt`, wantErr: runtime.GOOS == "windows"},
		{name: "drive relative path", path: `C:file.txt`, wantErr: runtime.GOOS == "windows"},
		{name: "drive absolute path", path: `C:\file.txt`, wantErr: runtime.GOOS == "windows"},
		{name: "UNC path", path: `\\server\share\file.txt`, wantErr: runtime.GOOS == "windows"},
		{name: "reserved Windows name", path: "NUL", wantErr: runtime.GOOS == "windows"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, validator := range validators {
				t.Run(validator.name, func(t *testing.T) {
					err := validator.validate(tt.path)
					if tt.wantErr {
						var traversalErr safepaths.PathTraversalError
						require.ErrorAs(t, err, &traversalErr)
						assert.Equal(t, []string{tt.path}, traversalErr.Elems)
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
}

func TestOpenFileRejectsTrailingSeparator(t *testing.T) {
	tests := []struct {
		name string
		path func(root string) string
	}{
		{
			name: "relative path",
			path: func(string) string {
				return "output" + string(os.PathSeparator)
			},
		},
		{
			name: "absolute path",
			path: func(root string) string {
				return filepath.Join(root, "output") + string(os.PathSeparator)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			path := tt.path(root)

			file, err := safepaths.OpenFile(path, 0o644, 0o755, false)
			if file != nil {
				_ = file.Close()
			}
			require.ErrorContains(t, err, "ends in a separator")
			_, statErr := os.Lstat(filepath.Join(root, "output"))
			require.True(t, os.IsNotExist(statErr))
			assert.NoFileExists(t, filepath.Join(root, "output", "output"))
		})
	}
}

func TestOpenFileAllowsTrustedParentSymlinks(t *testing.T) {
	tests := []struct {
		name string
		path func(sandbox string) string
	}{
		{
			name: "relative",
			path: func(string) string {
				return filepath.Join("link", "file.txt")
			},
		},
		{
			name: "absolute",
			path: func(sandbox string) string {
				return filepath.Join(sandbox, "link", "file.txt")
			},
		},
		{
			name: "parent relative",
			path: func(string) string {
				return filepath.Join("..", "link", "file.txt")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox := t.TempDir()
			target := filepath.Join(sandbox, "target")
			require.NoError(t, os.Mkdir(target, 0o755))
			require.NoError(t, os.Symlink(target, filepath.Join(sandbox, "link")))
			if tt.name == "parent relative" {
				work := filepath.Join(sandbox, "work")
				require.NoError(t, os.Mkdir(work, 0o755))
				t.Chdir(work)
			} else {
				t.Chdir(sandbox)
			}

			file, err := safepaths.OpenFile(tt.path(sandbox), 0o644, 0o755, false)
			require.NoError(t, err)
			_, err = file.Write([]byte("content"))
			require.NoError(t, err)
			require.NoError(t, file.Close())
			content, err := os.ReadFile(filepath.Join(target, "file.txt"))
			require.NoError(t, err)
			assert.Equal(t, "content", string(content))
		})
	}
}

func TestOpenRootDirAllowsTrustedPathSymlinks(t *testing.T) {
	tests := []struct {
		name string
		path func(sandbox string) string
	}{
		{
			name: "relative",
			path: func(string) string {
				return filepath.Join("link", "selected")
			},
		},
		{
			name: "absolute",
			path: func(sandbox string) string {
				return filepath.Join(sandbox, "link", "selected")
			},
		},
		{
			name: "parent relative",
			path: func(string) string {
				return filepath.Join("..", "link", "selected")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox := t.TempDir()
			target := filepath.Join(sandbox, "target")
			require.NoError(t, os.Mkdir(target, 0o755))
			require.NoError(t, os.Symlink(target, filepath.Join(sandbox, "link")))
			if tt.name == "parent relative" {
				work := filepath.Join(sandbox, "work")
				require.NoError(t, os.Mkdir(work, 0o755))
				t.Chdir(work)
			} else {
				t.Chdir(sandbox)
			}

			root, err := safepaths.OpenRootDir(tt.path(sandbox), 0o755)
			require.NoError(t, err)
			require.NoError(t, root.WriteFile("file.txt", []byte("content"), 0o644, 0o755, false))
			require.NoError(t, root.Close())
			content, err := os.ReadFile(filepath.Join(target, "selected", "file.txt"))
			require.NoError(t, err)
			assert.Equal(t, "content", string(content))
		})
	}
}
