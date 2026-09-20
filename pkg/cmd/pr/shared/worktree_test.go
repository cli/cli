package shared

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/run"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveWorktreeTarget(t *testing.T) {
	dir := t.TempDir()
	nonEmptyLinkedWorktree := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(nonEmptyLinkedWorktree, "file"), []byte("x"), 0o644))

	tests := []struct {
		name      string
		path      string
		stubs     func(*run.CommandStubber)
		wantReuse bool
		wantErr   string
	}{
		{
			name: "path is the current worktree",
			stubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 0, "/repo/main\n/repo/.git\n")
				cs.Register(`git -C .+rev-parse --path-format=absolute --show-toplevel --show-prefix --git-common-dir`, 0, "/repo/main\n\n/repo/.git\n")
			},
			wantErr: "--worktree path points to the repository you're already in; omit --worktree to check out here",
		},
		{
			name: "path is a subdirectory of the current worktree",
			stubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 0, "/repo/main\n/repo/.git\n")
				cs.Register(`git -C .+ rev-parse --path-format=absolute --show-toplevel --show-prefix --git-common-dir`, 0, "/repo/main\nsub/\n/repo/.git\n")
			},
			wantErr: "--worktree path points to the repository you're already in; omit --worktree to check out here",
		},
		{
			name: "path is a different worktree of this repo",
			path: nonEmptyLinkedWorktree,
			stubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 0, "/repo/main\n/repo/.git\n")
				cs.Register(`git -C .+ rev-parse --path-format=absolute --show-toplevel --show-prefix --git-common-dir`, 0, "/path/to/wt\n\n/repo/.git\n")
			},
			wantReuse: true,
		},
		{
			name: "path is a subdirectory of another worktree",
			stubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 0, "/repo/main\n/repo/.git\n")
				cs.Register(`git -C .+ rev-parse --path-format=absolute --show-toplevel --show-prefix --git-common-dir`, 0, "/path/to/wt\nsub/\n/repo/.git\n")
			},
			wantErr: "--worktree path is inside an existing worktree",
		},
		{
			name: "path is inside a different repository",
			stubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 0, "/repo/main\n/repo/.git\n")
				cs.Register(`git -C .+ rev-parse --path-format=absolute --show-toplevel --show-prefix --git-common-dir`, 0, "/other/wt\n\n/other/.git\n")
			},
			wantErr: "--worktree path is inside a different repository",
		},
		{
			name: "target is non-git or non-existent",
			stubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 0, "/repo/main\n/repo/.git\n")
				cs.Register(`git -C .+ rev-parse --path-format=absolute --show-toplevel --show-prefix --git-common-dir`, 128, "")
			},
		},
		{
			name: "current worktree cannot be determined",
			stubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 128, "")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, teardown := run.Stub()
			defer teardown(t)
			tt.stubs(cs)

			client := &git.Client{
				GhPath:  "/some/path/gh",
				GitPath: "/some/path/git",
			}
			path := tt.path
			if path == "" {
				path = dir
			}
			target, err := ResolveWorktreeTarget(client, path)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReuse, target.Reuse)
			assert.Equal(t, path, target.Path)
		})
	}
}

func TestResolveWorktreeTargetPathSafety(t *testing.T) {
	base := t.TempDir()

	existingDir := filepath.Join(base, "dir")
	require.NoError(t, os.Mkdir(existingDir, 0o755))

	nonEmptyDir := filepath.Join(base, "non-empty-dir")
	require.NoError(t, os.Mkdir(nonEmptyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nonEmptyDir, "file"), []byte("x"), 0o644))

	regularFile := filepath.Join(base, "file")
	require.NoError(t, os.WriteFile(regularFile, []byte("x"), 0o644))

	symlink := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(existingDir, symlink))

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name: "non-existent path is allowed",
			path: filepath.Join(base, "does-not-exist"),
		},
		{
			name: "existing directory is allowed",
			path: existingDir,
		},
		{
			name:    "existing non-empty directory is rejected",
			path:    nonEmptyDir,
			wantErr: "--worktree path must be empty",
		},
		{
			name:    "leaf symlink is rejected",
			path:    symlink,
			wantErr: "--worktree path must not be a symlink",
		},
		{
			name:    "existing non-directory is rejected",
			path:    regularFile,
			wantErr: "--worktree path must be a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, teardown := run.Stub()
			defer teardown(t)
			if tt.wantErr == "" || tt.path == nonEmptyDir {
				cs.Register(`git rev-parse --path-format=absolute --show-toplevel --git-common-dir`, 128, "")
			}

			client := &git.Client{GitPath: "/some/path/git"}
			_, err := ResolveWorktreeTarget(client, tt.path)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestWorktreeCheckoutCommands(t *testing.T) {
	tests := []struct {
		name         string
		target       WorktreeTarget
		branchExists bool
		wantCommands [][]string
	}{
		{
			name:         "fresh target with existing branch",
			target:       WorktreeTarget{Path: "/path/to/wt"},
			branchExists: true,
			wantCommands: [][]string{{"worktree", "add", "--", "/path/to/wt", "feature"}},
		},
		{
			name:         "fresh target with new branch",
			target:       WorktreeTarget{Path: "/path/to/wt"},
			wantCommands: [][]string{{"worktree", "add", "--track", "-b", "feature", "--", "/path/to/wt", "origin/feature"}},
		},
		{
			name:         "reused target with existing branch",
			target:       WorktreeTarget{Path: "/path/to/wt", Reuse: true},
			branchExists: true,
			wantCommands: [][]string{{"-C", "/path/to/wt", "checkout", "feature"}},
		},
		{
			name:         "reused target with new branch",
			target:       WorktreeTarget{Path: "/path/to/wt", Reuse: true},
			wantCommands: [][]string{{"-C", "/path/to/wt", "checkout", "-b", "feature", "--track", "origin/feature"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, teardown := run.Stub()
			defer teardown(t)
			exitCode := 1
			if tt.branchExists {
				exitCode = 0
			}
			cs.Register(`git rev-parse --verify refs/heads/feature`, exitCode, "")

			client := &git.Client{GitPath: "/some/path/git"}
			commands, branchExists := WorktreeCheckoutCommands(client, tt.target, "feature", "origin/feature")

			assert.Equal(t, tt.branchExists, branchExists)
			assert.Equal(t, tt.wantCommands, commands)
		})
	}
}
