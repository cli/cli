package cmdutil

import (
	"os"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func Test_EnableRepoOverride(t *testing.T) {
	originalGhRepo := os.Getenv("GH_REPO")
	originalGithubRepository := os.Getenv("GITHUB_REPOSITORY")
	t.Cleanup(func() {
		_ = os.Setenv("GH_REPO", originalGhRepo)
		_ = os.Setenv("GITHUB_REPOSITORY", originalGithubRepository)
	})

	tests := []struct {
		name     string
		gh       string
		github   string
		expected string
	}{
		{
			name:     "GH_REPO",
			gh:       "gh/repo",
			github:   "",
			expected: "gh/repo",
		},
		{
			name:     "GITHUB_REPOSITORY",
			gh:       "",
			github:   "github/repository",
			expected: "github/repository",
		},
		{
			name:     "GH_REPO, GITHUB_REPOSITORY",
			gh:       "gh/repo",
			github:   "github/repository",
			expected: "gh/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Setenv("GH_REPO", tt.gh)
			_ = os.Setenv("GITHUB_REPOSITORY", tt.github)

			factory := &Factory{}
			cmd := &cobra.Command{Run: func(*cobra.Command, []string) {}}
			EnableRepoOverride(cmd, factory)

			err := cmd.Execute()
			assert.NoError(t, err)

			repo, err := factory.BaseRepo()
			assert.NoError(t, err)

			assert.Equal(t, tt.expected, ghrepo.FullName(repo))
		})
	}
}
