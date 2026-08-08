package installer

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/registry"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallLocal(t *testing.T) {
	tests := []struct {
		name         string
		skills       []discovery.Skill
		useAgentHost bool
		setup        func(t *testing.T, srcDir string)
		verify       func(t *testing.T, destDir string)
		wantErr      string
	}{
		{
			name:   "copies files via Dir",
			skills: []discovery.Skill{{Name: "code-review", Path: "skills/code-review"}},
			setup: func(t *testing.T, srcDir string) {
				t.Helper()
				skillSrc := filepath.Join(srcDir, "skills", "code-review")
				require.NoError(t, os.MkdirAll(skillSrc, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("# Code Review"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(skillSrc, "prompt.txt"), []byte("review this PR"), 0o644))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(destDir, "code-review", "prompt.txt"))
				require.NoError(t, err)
				assert.Equal(t, "review this PR", string(content))

				_, err = os.Stat(filepath.Join(destDir, "code-review", "SKILL.md"))
				assert.NoError(t, err)
			},
		},
		{
			name:   "nested directories",
			skills: []discovery.Skill{{Name: "issue-triage", Path: "skills/issue-triage"}},
			setup: func(t *testing.T, srcDir string) {
				t.Helper()
				deep := filepath.Join(srcDir, "skills", "issue-triage", "prompts", "templates")
				require.NoError(t, os.MkdirAll(deep, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(deep, "bug.txt"), []byte("triage bug"), 0o644))
				require.NoError(t, os.WriteFile(
					filepath.Join(srcDir, "skills", "issue-triage", "SKILL.md"), []byte("# Issue Triage"), 0o644))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(destDir, "issue-triage", "prompts", "templates", "bug.txt"))
				require.NoError(t, err)
				assert.Equal(t, "triage bug", string(content))
			},
		},
		{
			name:   "skips symlinks",
			skills: []discovery.Skill{{Name: "pr-summary", Path: "skills/pr-summary"}},
			setup: func(t *testing.T, srcDir string) {
				t.Helper()
				skillSrc := filepath.Join(srcDir, "skills", "pr-summary")
				require.NoError(t, os.MkdirAll(skillSrc, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("# PR Summary"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(skillSrc, "prompt.txt"), []byte("summarize"), 0o644))
				require.NoError(t, os.Symlink(filepath.Join(skillSrc, "prompt.txt"), filepath.Join(skillSrc, "link.txt")))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(destDir, "pr-summary", "prompt.txt"))
				assert.NoError(t, err)
				_, err = os.Stat(filepath.Join(destDir, "pr-summary", "link.txt"))
				assert.True(t, os.IsNotExist(err))
			},
		},
		{
			name:   "injects metadata into SKILL.md",
			skills: []discovery.Skill{{Name: "copilot-helper", Path: "skills/copilot-helper"}},
			setup: func(t *testing.T, srcDir string) {
				t.Helper()
				skillSrc := filepath.Join(srcDir, "skills", "copilot-helper")
				require.NoError(t, os.MkdirAll(skillSrc, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("# Copilot Helper\nAssists with tasks"), 0o644))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(destDir, "copilot-helper", "SKILL.md"))
				require.NoError(t, err)
				assert.Contains(t, string(content), "local-path")
			},
		},
		{
			name: "multiple skills",
			skills: []discovery.Skill{
				{Name: "code-review", Path: "skills/code-review"},
				{Name: "issue-triage", Path: "skills/issue-triage"},
			},
			setup: func(t *testing.T, srcDir string) {
				t.Helper()
				for _, name := range []string{"code-review", "issue-triage"} {
					skillSrc := filepath.Join(srcDir, "skills", name)
					require.NoError(t, os.MkdirAll(skillSrc, 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("# "+name), 0o644))
				}
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(destDir, "code-review", "SKILL.md"))
				assert.NoError(t, err)
				_, err = os.Stat(filepath.Join(destDir, "issue-triage", "SKILL.md"))
				assert.NoError(t, err)
			},
		},
		{
			name:         "resolves install dir from AgentHost and Scope",
			skills:       []discovery.Skill{{Name: "code-review", Path: "skills/code-review"}},
			useAgentHost: true,
			setup: func(t *testing.T, srcDir string) {
				t.Helper()
				skillSrc := filepath.Join(srcDir, "skills", "code-review")
				require.NoError(t, os.MkdirAll(skillSrc, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillSrc, "SKILL.md"), []byte("# Code Review"), 0o644))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(destDir, ".agents", "skills", "code-review", "SKILL.md"))
				assert.NoError(t, err)
			},
		},
		{
			name:    "no dir or agent host",
			skills:  []discovery.Skill{{Name: "code-review"}},
			setup:   func(t *testing.T, srcDir string) {},
			wantErr: "either Dir or AgentHost must be specified",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcDir := t.TempDir()
			destDir := t.TempDir()
			tt.setup(t, srcDir)

			opts := &LocalOptions{
				SourceDir: srcDir,
				Skills:    tt.skills,
				Dir:       destDir,
			}
			if tt.useAgentHost {
				host, err := registry.FindByID("github-copilot")
				require.NoError(t, err)
				opts.Dir = ""
				opts.AgentHost = host
				opts.Scope = registry.ScopeProject
				opts.GitRoot = destDir
			}
			if tt.wantErr != "" {
				opts.Dir = ""
			}

			result, err := InstallLocal(opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, result.Dir)
			assert.Len(t, result.Installed, len(tt.skills))
			tt.verify(t, destDir)
		})
	}
}

func TestInstallSkill(t *testing.T) {
	tests := []struct {
		name   string
		skill  discovery.Skill
		stubs  func(*httpmock.Registry)
		verify func(t *testing.T, destDir string)
	}{
		{
			name:  "installs files from remote",
			skill: discovery.Skill{Name: "code-review", Path: "skills/code-review", TreeSHA: "tree123"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "skill-sha", "size": 10},
							{"path": "prompt.txt", "type": "blob", "sha": "prompt-sha", "size": 5},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/skill-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "skill-sha", "encoding": "base64",
						"content": base64.StdEncoding.EncodeToString([]byte("# Code Review")),
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/prompt-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "prompt-sha", "encoding": "base64",
						"content": base64.StdEncoding.EncodeToString([]byte("review this PR")),
					}))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(destDir, "code-review", "prompt.txt"))
				require.NoError(t, err)
				assert.Equal(t, "review this PR", string(content))

				_, err = os.Stat(filepath.Join(destDir, "code-review", "SKILL.md"))
				assert.NoError(t, err)
			},
		},
		{
			name:  "injects metadata into SKILL.md",
			skill: discovery.Skill{Name: "pr-summary", Path: "skills/pr-summary", TreeSHA: "tree456"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree456"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree456", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "md-sha", "size": 20},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/md-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "md-sha", "encoding": "base64",
						"content": base64.StdEncoding.EncodeToString([]byte("# PR Summary\nSummarize pull requests")),
					}))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(destDir, "pr-summary", "SKILL.md"))
				require.NoError(t, err)
				assert.NotContains(t, string(content), "github-owner:")
				assert.Contains(t, string(content), "github-repo: https://github.com/monalisa/octocat-skills")
			},
		},
		{
			name:  "fails on path traversal from malicious tree",
			skill: discovery.Skill{Name: "code-review", Path: "skills/code-review", TreeSHA: "tree123"},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "safe-sha", "size": 10},
							{"path": "../../etc/passwd", "type": "blob", "sha": "evil-sha", "size": 100},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/safe-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "safe-sha", "encoding": "base64",
						"content": base64.StdEncoding.EncodeToString([]byte("# Safe Skill")),
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/evil-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "evil-sha", "encoding": "base64",
						"content": base64.StdEncoding.EncodeToString([]byte("malicious content")),
					}))
			},
			verify: func(t *testing.T, destDir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(destDir, "..", "etc", "passwd"))
				assert.True(t, os.IsNotExist(err), "traversal path should not be written")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destDir := t.TempDir()
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})
			opts := &Options{
				Host:   "github.com",
				Owner:  "monalisa",
				Repo:   "octocat-skills",
				Ref:    "v1.0",
				SHA:    "commit123",
				Client: client,
			}

			err := installSkill(opts, tt.skill, destDir)
			if tt.name == "fails on path traversal from malicious tree" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "blocked path traversal")
			} else {
				require.NoError(t, err)
			}
			tt.verify(t, destDir)
		})
	}
}

func stubTreeAndBlob(reg *httpmock.Registry, treeSHA string) {
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/monalisa/octocat-skills/git/trees/%s", treeSHA)),
		httpmock.JSONResponse(map[string]interface{}{
			"sha": treeSHA, "truncated": false,
			"tree": []map[string]interface{}{
				{"path": "SKILL.md", "type": "blob", "sha": treeSHA + "-blob", "size": 10},
			},
		}))
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/monalisa/octocat-skills/git/blobs/%s-blob", treeSHA)),
		httpmock.JSONResponse(map[string]interface{}{
			"sha": treeSHA + "-blob", "encoding": "base64",
			"content": base64.StdEncoding.EncodeToString([]byte("# Skill")),
		}))
}

func TestInstall(t *testing.T) {
	var progressCount atomic.Int32

	tests := []struct {
		name          string
		skills        []discovery.Skill
		stubs         func(*httpmock.Registry)
		onProgress    func(done, total int)
		wantInstalled []string
		wantErr       string
	}{
		{
			name: "single skill calls OnProgress",
			skills: []discovery.Skill{
				{Name: "code-review", Path: "skills/code-review", TreeSHA: "tree-cr"},
			},
			stubs: func(reg *httpmock.Registry) { stubTreeAndBlob(reg, "tree-cr") },
			onProgress: func(done, total int) {

				progressCount.Add(1)

			},
			wantInstalled: []string{"code-review"},
		},
		{
			name: "multiple skills concurrently with progress",
			skills: []discovery.Skill{
				{Name: "code-review", Path: "skills/code-review", TreeSHA: "tree-cr"},
				{Name: "issue-triage", Path: "skills/issue-triage", TreeSHA: "tree-it"},
			},
			stubs: func(reg *httpmock.Registry) {
				stubTreeAndBlob(reg, "tree-cr")
				stubTreeAndBlob(reg, "tree-it")
			},
			onProgress: func(done, total int) {

				progressCount.Add(1)

			},
			wantInstalled: []string{"code-review", "issue-triage"},
		},
		{
			name: "partial failure returns successful installs and error",
			skills: []discovery.Skill{
				{Name: "code-review", Path: "skills/code-review", TreeSHA: "tree-cr"},
				{Name: "issue-triage", Path: "skills/issue-triage", TreeSHA: "tree-fail"},
			},
			stubs: func(reg *httpmock.Registry) {
				stubTreeAndBlob(reg, "tree-cr")
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree-fail"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantInstalled: []string{"code-review"},
			wantErr:       "failed to install skill",
		},
		{
			name:    "no dir or agent host",
			skills:  []discovery.Skill{{Name: "code-review"}},
			stubs:   func(reg *httpmock.Registry) {},
			wantErr: "either Dir or AgentHost must be specified",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progressCount.Store(0)
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)

			destDir := t.TempDir()
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			opts := &Options{
				Host:       "github.com",
				Owner:      "monalisa",
				Repo:       "octocat-skills",
				Ref:        "v1.0",
				SHA:        "commit123",
				Client:     client,
				Skills:     tt.skills,
				Dir:        destDir,
				OnProgress: tt.onProgress,
			}
			if tt.wantErr != "" && len(tt.wantInstalled) == 0 {
				opts.Dir = ""
			}

			result, err := Install(opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				if len(tt.wantInstalled) > 0 {
					require.NotNil(t, result, "partial failure should return non-nil result")
					assert.ElementsMatch(t, tt.wantInstalled, result.Installed)
				}
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.wantInstalled, result.Installed)
			assert.Equal(t, destDir, result.Dir)

			homeDir, _ = os.UserHomeDir()
			lockPath := filepath.Join(homeDir, ".agents", ".skill-lock.json")
			lockData, err := os.ReadFile(lockPath)
			require.NoError(t, err, "lockfile should have been written")
			for _, name := range tt.wantInstalled {
				assert.Contains(t, string(lockData), name)
			}
			if tt.onProgress != nil {
				assert.True(t, progressCount.Load() > 0, "OnProgress should have been called")
			}
		})
	}
}

func TestInstallSingleSkillFailureStillCompletesProgress(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	destDir := t.TempDir()
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree-fail"),
		httpmock.StatusStringResponse(500, "server error"),
	)
	client := api.NewClientFromHTTP(&http.Client{Transport: reg})

	var events []struct{ done, total int }
	result, err := Install(&Options{
		Host:   "github.com",
		Owner:  "monalisa",
		Repo:   "octocat-skills",
		Ref:    "v1.0",
		SHA:    "commit123",
		Client: client,
		Skills: []discovery.Skill{
			{Name: "code-review", Path: "skills/code-review", TreeSHA: "tree-fail"},
		},
		Dir: destDir,
		OnProgress: func(done, total int) {
			events = append(events, struct{ done, total int }{done: done, total: total})
		},
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, []struct{ done, total int }{{done: 0, total: 1}, {done: 1, total: 1}}, events)
}

func TestResolveGitRoot(t *testing.T) {
	tests := []struct {
		name    string
		client  *git.Client
		wantDir string
	}{
		{
			name:    "returns RepoDir when set",
			client:  &git.Client{RepoDir: "/monalisa/repo"},
			wantDir: "/monalisa/repo",
		},
		{
			name:   "nil client falls back to cwd",
			client: nil,
		},
		{
			name:   "empty RepoDir falls back to ToplevelDir or cwd",
			client: &git.Client{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveGitRoot(tt.client)
			if tt.wantDir != "" {
				assert.Equal(t, tt.wantDir, got)
			} else {
				assert.NotEmpty(t, got, "should fall back to ToplevelDir or cwd")
			}
		})
	}
}
