package registry

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindByID(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		wantName string
		wantErr  string
	}{
		{name: "github-copilot", id: "github-copilot", wantName: "GitHub Copilot"},
		{name: "claude-code", id: "claude-code", wantName: "Claude Code"},
		{name: "cursor", id: "cursor", wantName: "Cursor"},
		{name: "codex", id: "codex", wantName: "Codex"},
		{name: "gemini-cli", id: "gemini-cli", wantName: "Gemini CLI"},
		{name: "antigravity", id: "antigravity", wantName: "Antigravity"},
		{name: "unknown agent", id: "nonexistent", wantErr: "unknown agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := FindByID(tt.id)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, host.Name)
		})
	}
}

func TestInstallDir(t *testing.T) {
	tests := []struct {
		name    string
		hostID  string
		scope   Scope
		gitRoot string
		homeDir string
		wantDir string
		wantErr bool
	}{
		{
			name:    "github copilot project scope",
			hostID:  "github-copilot",
			scope:   ScopeProject,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantDir: filepath.Join("/tmp/monalisa-repo", ".agents", "skills"),
		},
		{
			name:    "github copilot user scope",
			hostID:  "github-copilot",
			scope:   ScopeUser,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantDir: filepath.Join("/home/monalisa", ".copilot", "skills"),
		},
		{
			name:    "claude code project scope",
			hostID:  "claude-code",
			scope:   ScopeProject,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantDir: filepath.Join("/tmp/monalisa-repo", ".claude", "skills"),
		},
		{
			name:    "cursor project scope",
			hostID:  "cursor",
			scope:   ScopeProject,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantDir: filepath.Join("/tmp/monalisa-repo", ".agents", "skills"),
		},
		{
			name:    "codex project scope",
			hostID:  "codex",
			scope:   ScopeProject,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantDir: filepath.Join("/tmp/monalisa-repo", ".agents", "skills"),
		},
		{
			name:    "gemini project scope",
			hostID:  "gemini-cli",
			scope:   ScopeProject,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantDir: filepath.Join("/tmp/monalisa-repo", ".agents", "skills"),
		},
		{
			name:    "antigravity project scope",
			hostID:  "antigravity",
			scope:   ScopeProject,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantDir: filepath.Join("/tmp/monalisa-repo", ".agents", "skills"),
		},
		{
			name:    "project scope without git root",
			hostID:  "github-copilot",
			scope:   ScopeProject,
			gitRoot: "",
			homeDir: "/home/monalisa",
			wantErr: true,
		},
		{
			name:    "user scope without home dir",
			hostID:  "github-copilot",
			scope:   ScopeUser,
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "",
			wantErr: true,
		},
		{
			name:    "invalid scope",
			hostID:  "github-copilot",
			scope:   "bogus",
			gitRoot: "/tmp/monalisa-repo",
			homeDir: "/home/monalisa",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := FindByID(tt.hostID)
			require.NoError(t, err)

			dir, err := host.InstallDir(tt.scope, tt.gitRoot, tt.homeDir)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestRepoNameFromRemote(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"https://github.com/monalisa/octocat-skills.git", "monalisa/octocat-skills"},
		{"https://github.com/monalisa/octocat-skills", "monalisa/octocat-skills"},
		{"git@github.com:monalisa/octocat-skills.git", "monalisa/octocat-skills"},
		{"git@github.com:monalisa/octocat-skills", "monalisa/octocat-skills"},
		{"ssh://git@github.com/monalisa/octocat-skills.git", "monalisa/octocat-skills"},
		{"ssh://git@github.com/monalisa/octocat-skills", "monalisa/octocat-skills"},
		{"not-a-url", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.remote, func(t *testing.T) {
			assert.Equal(t, tt.want, RepoNameFromRemote(tt.remote))
		})
	}
}

func TestUniqueProjectDirs(t *testing.T) {
	dirs := UniqueProjectDirs()
	seen := map[string]int{}
	for _, d := range dirs {
		seen[d]++
	}
	// The shared .agents/skills dir and .claude/skills must both be present
	// and listed exactly once each.
	assert.Equal(t, 1, seen[".agents/skills"], "expected .agents/skills exactly once")
	assert.Equal(t, 1, seen[".claude/skills"], "expected .claude/skills exactly once")
	// No project dir should appear more than once.
	for d, n := range seen {
		assert.LessOrEqualf(t, n, 1, "project dir %q appears %d times", d, n)
	}
}

func TestScopeLabels(t *testing.T) {
	tests := []struct {
		name       string
		repoName   string
		wantFirst  []string
		wantSecond []string
	}{
		{
			name:       "without repo name",
			repoName:   "",
			wantFirst:  []string{"Project", "recommended"},
			wantSecond: []string{"Global"},
		},
		{
			name:       "with repo name",
			repoName:   "monalisa/octocat-skills",
			wantFirst:  []string{"monalisa/octocat-skills", "recommended"},
			wantSecond: []string{"Global"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := ScopeLabels(tt.repoName)
			require.Len(t, labels, 2)
			for _, s := range tt.wantFirst {
				assert.Contains(t, labels[0], s)
			}
			for _, s := range tt.wantSecond {
				assert.Contains(t, labels[1], s)
			}
		})
	}
}
