package sync

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
)

func Test_worktreePathForBranch(t *testing.T) {
	// Representative `git worktree list --porcelain` output covering a bare main
	// worktree, a detached worktree, and a branch whose name prefixes another.
	porcelain := heredoc.Doc(`
		worktree /repos/project.git
		bare

		worktree /repos/primary
		HEAD 8f0dc4bbde2d1d10e73b0cd0b1d3f7c1f1f5b0aa
		branch refs/heads/trunk

		worktree /repos/detached
		HEAD 3c1d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d
		detached

		worktree /repos/feature work
		HEAD 1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b
		branch refs/heads/feature/one
	`)

	tests := []struct {
		name      string
		porcelain string
		branch    string
		want      string
	}{
		{
			name:      "branch checked out in a worktree",
			porcelain: porcelain,
			branch:    "trunk",
			want:      "/repos/primary",
		},
		{
			name:      "branch with slashes and a path containing a space",
			porcelain: porcelain,
			branch:    "feature/one",
			want:      "/repos/feature work",
		},
		{
			name:      "branch not checked out anywhere",
			porcelain: porcelain,
			branch:    "other",
			want:      "",
		},
		{
			name:      "branch name that only prefixes a checked out branch",
			porcelain: porcelain,
			branch:    "feature",
			want:      "",
		},
		{
			name:      "no worktrees reported",
			porcelain: "",
			branch:    "trunk",
			want:      "",
		},
		{
			name:      "windows line endings",
			porcelain: "worktree /repos/primary\r\nHEAD 8f0dc4b\r\nbranch refs/heads/trunk\r\n",
			branch:    "trunk",
			want:      "/repos/primary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, worktreePathForBranch(tt.porcelain, tt.branch))
		})
	}
}
