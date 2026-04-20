package install

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/skills/discovery"
	"github.com/cli/cli/v2/internal/skills/registry"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdInstall(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wantOpts      InstallOptions
		wantLocalPath bool
		wantErr       bool
	}{
		{
			name:     "repo argument only",
			cli:      "monalisa/skills-repo",
			wantOpts: InstallOptions{SkillSource: "monalisa/skills-repo", Scope: "project"},
		},
		{
			name:     "repo and skill",
			cli:      "monalisa/skills-repo git-commit",
			wantOpts: InstallOptions{SkillSource: "monalisa/skills-repo", SkillName: "git-commit", Scope: "project"},
		},
		{
			name: "all flags",
			cli:  "monalisa/skills-repo git-commit --agent github-copilot --scope user --pin v1.0.0 --force",
			wantOpts: InstallOptions{
				SkillSource: "monalisa/skills-repo",
				SkillName:   "git-commit",
				Agent:       "github-copilot",
				Scope:       "user",
				Pin:         "v1.0.0",
				Force:       true,
			},
		},
		{
			name:     "dir flag",
			cli:      "monalisa/skills-repo git-commit --dir ./custom-skills",
			wantOpts: InstallOptions{SkillSource: "monalisa/skills-repo", SkillName: "git-commit", Dir: "./custom-skills", Scope: "project"},
		},
		{
			name:    "too many args",
			cli:     "a b c",
			wantErr: true,
		},
		{
			name:    "invalid agent flag",
			cli:     "monalisa/skills-repo git-commit --agent nonexistent",
			wantErr: true,
		},
		{
			name:    "pin conflicts with inline version",
			cli:     "monalisa/skills-repo git-commit@v1.0.0 --pin v2.0.0",
			wantErr: true,
		},
		{
			name:     "alias add works",
			cli:      "monalisa/skills-repo git-commit",
			wantOpts: InstallOptions{SkillSource: "monalisa/skills-repo", SkillName: "git-commit", Scope: "project"},
		},
		{
			name:          "from-local flag sets localPath",
			cli:           "--from-local ./local-dir",
			wantOpts:      InstallOptions{SkillSource: "./local-dir", Scope: "project", FromLocal: true},
			wantLocalPath: true,
		},
		{
			name:          "from-local with absolute path",
			cli:           "--from-local /absolute/path",
			wantOpts:      InstallOptions{SkillSource: "/absolute/path", Scope: "project", FromLocal: true},
			wantLocalPath: true,
		},
		{
			name:          "from-local with tilde path",
			cli:           "--from-local ~/skills",
			wantOpts:      InstallOptions{SkillSource: "~/skills", Scope: "project", FromLocal: true},
			wantLocalPath: true,
		},
		{
			name:     "owner/repo does not set localPath",
			cli:      "monalisa/skills-repo",
			wantOpts: InstallOptions{SkillSource: "monalisa/skills-repo", Scope: "project"},
		},
		{
			name:     "local-looking path without --from-local treated as repo",
			cli:      "./local-dir",
			wantOpts: InstallOptions{SkillSource: "./local-dir", Scope: "project"},
		},
		{
			name:    "from-local without argument errors",
			cli:     "--from-local",
			wantErr: true,
		},
		{
			name:    "from-local with --pin is mutually exclusive",
			cli:     "--from-local ./local-dir --pin v1.0.0",
			wantErr: true,
		},
		{
			name:     "allow-hidden-dirs flag",
			cli:      "monalisa/skills-repo --allow-hidden-dirs",
			wantOpts: InstallOptions{SkillSource: "monalisa/skills-repo", Scope: "project", AllowHiddenDirs: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Prompter:  &prompter.PrompterMock{},
				GitClient: &git.Client{},
			}

			var gotOpts *InstallOptions
			cmd := NewCmdInstall(f, &telemetry.NoOpService{}, func(opts *InstallOptions) error {
				gotOpts = opts
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err = cmd.Execute()
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, gotOpts)
			assert.Equal(t, tt.wantOpts.SkillSource, gotOpts.SkillSource)
			assert.Equal(t, tt.wantOpts.SkillName, gotOpts.SkillName)
			assert.Equal(t, tt.wantOpts.Agent, gotOpts.Agent)
			assert.Equal(t, tt.wantOpts.Scope, gotOpts.Scope)
			assert.Equal(t, tt.wantOpts.Pin, gotOpts.Pin)
			assert.Equal(t, tt.wantOpts.Dir, gotOpts.Dir)
			assert.Equal(t, tt.wantOpts.Force, gotOpts.Force)
			assert.Equal(t, tt.wantOpts.FromLocal, gotOpts.FromLocal)
			assert.Equal(t, tt.wantOpts.AllowHiddenDirs, gotOpts.AllowHiddenDirs)
			if tt.wantLocalPath {
				assert.NotEmpty(t, gotOpts.localPath, "expected localPath to be set")
			} else {
				assert.Empty(t, gotOpts.localPath, "expected localPath to be empty")
			}
		})
	}

	// Verify command metadata separately.
	t.Run("command metadata", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()
		f := &cmdutil.Factory{IOStreams: ios, Prompter: &prompter.PrompterMock{}, GitClient: &git.Client{}}
		cmd := NewCmdInstall(f, &telemetry.NoOpService{}, nil)

		assert.Equal(t, "install <repository> [<skill[@version]>] [flags]", cmd.Use)
		assert.NotEmpty(t, cmd.Short)
		assert.NotEmpty(t, cmd.Long)
		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Aliases, "add")

		for _, flag := range []string{"agent", "scope", "pin", "dir", "force"} {
			assert.NotNil(t, cmd.Flags().Lookup(flag), "missing flag: --%s", flag)
		}
	})
}

// --- HTTP stub helpers ---

// stubResolveVersion registers API stubs for latest release + tag resolution.
func stubResolveVersion(reg *httpmock.Registry, owner, repo, tag, sha string) {
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/%s/%s/releases/latest", owner, repo)),
		httpmock.StringResponse(fmt.Sprintf(`{"tag_name": %q}`, tag)),
	)
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/%s/%s/git/ref/tags/%s", owner, repo, tag)),
		httpmock.StringResponse(fmt.Sprintf(`{"object": {"sha": %q, "type": "commit"}}`, sha)),
	)
}

// stubDiscoverTree registers the single recursive-tree call used by DiscoverSkills.
func stubDiscoverTree(reg *httpmock.Registry, owner, repo, sha, treeJSON string) {
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/%s/%s/git/trees/%s", owner, repo, sha)),
		httpmock.StringResponse(fmt.Sprintf(`{"sha": %q, "tree": [%s]}`, sha, treeJSON)),
	)
}

// stubInstallFiles registers subtree + blob stubs for installer.Install (one skill).
func stubInstallFiles(reg *httpmock.Registry, owner, repo, treeSHA, blobSHA, content string) {
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/%s/%s/git/trees/%s", owner, repo, treeSHA)),
		httpmock.StringResponse(fmt.Sprintf(`{"tree": [{"path": "SKILL.md", "type": "blob", "sha": %q, "size": 50}]}`, blobSHA)),
	)
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/%s/%s/git/blobs/%s", owner, repo, blobSHA)),
		httpmock.StringResponse(fmt.Sprintf(`{"sha": %q, "content": %q, "encoding": "base64"}`, blobSHA, encoded)),
	)
}

// stubSkillByPath registers stubs for DiscoverSkillByPath (contents API + tree).
func stubSkillByPath(reg *httpmock.Registry, owner, repo, sha, skillPath, skillName, treeSHA string) {
	parentPath := skillPath
	if idx := strings.LastIndex(skillPath, "/"); idx >= 0 {
		parentPath = skillPath[:idx]
	}
	reg.Register(
		httpmock.REST("GET", fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, parentPath)),
		httpmock.StringResponse(fmt.Sprintf(`[{"name": %q, "path": %q, "sha": %q, "type": "dir"}]`, skillName, skillPath, treeSHA)),
	)
}

// writeLocalTestSkill creates a skill directory with a SKILL.md file.
func writeLocalTestSkill(t *testing.T, baseDir, subPath, content string) {
	t.Helper()
	skillDir := filepath.Join(baseDir, subPath)
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))
}

// --- Skill content constants ---

var gitCommitContent = heredoc.Doc(`
	---
	name: git-commit
	description: Writes commits
	---
	# Git Commit
`)

// singleSkillTreeJSON returns tree entries for a single skill with the given name.
func singleSkillTreeJSON(name, treeSHA, blobSHA string) string {
	return fmt.Sprintf(
		`{"path": "skills/%s", "type": "tree", "sha": %q}, {"path": "skills/%s/SKILL.md", "type": "blob", "sha": %q}`,
		name, treeSHA, name, blobSHA,
	)
}

// hiddenDirSkillTreeJSON returns tree entries for a hidden-dir skill under .claude/skills/.
func hiddenDirSkillTreeJSON(name, treeSHA, blobSHA string) string {
	return fmt.Sprintf(
		`{"path": ".claude/skills/%s", "type": "tree", "sha": %q}, {"path": ".claude/skills/%s/SKILL.md", "type": "blob", "sha": %q}`,
		name, treeSHA, name, blobSHA,
	)
}

func TestInstallRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		setup      func(t *testing.T)
		stubs      func(*httpmock.Registry)
		opts       func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions
		verify     func(t *testing.T)
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:  "non-interactive without repo errors",
			isTTY: false,
			opts: func(ios *iostreams.IOStreams, _ *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:        ios,
					GitClient: &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantErr: "must specify a repository to install from",
		},
		{
			name:  "non-interactive without skill name errors",
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
				}
			},
			wantErr: "must specify a skill name when not running interactively",
		},
		{
			name:  "remote install writes files with tracking metadata",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install with --agent claude-code",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "claude-code",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install defaults to github-copilot non-interactively",
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install with --scope user",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "user",
					ScopeChanged: true,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install with --dir bypasses scope resolution",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:          ios,
					HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "monalisa/skills-repo",
					SkillName:   "git-commit",
					Agent:       "github-copilot",
					Dir:         t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install with --force overwrites existing skill",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				targetDir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "git-commit"), 0o755))
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					Force:        true,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install existing skill without force non-interactive errors",
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				targetDir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "git-commit"), 0o755))
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
				}
			},
			wantErr: "already installed",
		},
		{
			name:  "remote install skill not found errors",
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "nonexistent",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantErr: `skill "nonexistent" not found`,
		},
		{
			name:  "remote install ambiguous skill name errors",
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				// Two namespaced skills with the same name
				treeJSON := `{"path": "skills/alice", "type": "tree", "sha": "nsA"}, ` +
					`{"path": "skills/alice/xlsx-pro", "type": "tree", "sha": "treeA"}, ` +
					`{"path": "skills/alice/xlsx-pro/SKILL.md", "type": "blob", "sha": "blobA"}, ` +
					`{"path": "skills/bob", "type": "tree", "sha": "nsB"}, ` +
					`{"path": "skills/bob/xlsx-pro", "type": "tree", "sha": "treeB"}, ` +
					`{"path": "skills/bob/xlsx-pro/SKILL.md", "type": "blob", "sha": "blobB"}`
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123", treeJSON)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "xlsx-pro",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantErr: "ambiguous",
		},
		{
			name:  "remote install namespaced exact match resolves ambiguity",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				treeJSON := `{"path": "skills/alice", "type": "tree", "sha": "nsA"}, ` +
					`{"path": "skills/alice/xlsx-pro", "type": "tree", "sha": "treeA"}, ` +
					`{"path": "skills/alice/xlsx-pro/SKILL.md", "type": "blob", "sha": "blobA"}, ` +
					`{"path": "skills/bob", "type": "tree", "sha": "nsB"}, ` +
					`{"path": "skills/bob/xlsx-pro", "type": "tree", "sha": "treeB"}, ` +
					`{"path": "skills/bob/xlsx-pro/SKILL.md", "type": "blob", "sha": "blobB"}`
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123", treeJSON)
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeB", "blobB",
					"---\nname: xlsx-pro\ndescription: Bob version\n---\n# B\n")
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "bob/xlsx-pro",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed bob/xlsx-pro",
		},
		{
			name:  "remote install with invalid repo argument errors",
			isTTY: false,
			opts: func(ios *iostreams.IOStreams, _ *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:          ios,
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "invalid",
					SkillName:   "git-commit",
				}
			},
			wantErr: "invalid repository reference",
		},
		{
			name:  "remote install with pin flag resolves version",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/git/ref/heads/v2.0.0"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/git/ref/tags/v2.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "def456", "type": "commit"}}`),
				)
				stubDiscoverTree(reg, "monalisa", "skills-repo", "def456",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Pin:          "v2.0.0",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
			wantStderr: "v2.0.0",
		},
		{
			name:  "remote install shows pre-install disclaimer",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
			wantStderr: "not verified by GitHub",
		},
		{
			name:  "remote install outputs review hint",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
			wantStderr: "gh skill preview monalisa/skills-repo git-commit@abc123",
		},
		{
			name:  "remote install outputs file tree for TTY",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStderr: "SKILL.md",
		},
		{
			name:  "remote install with inline version parses name and version",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/git/ref/heads/v1.2.0"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/git/ref/tags/v1.2.0"),
					httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
				)
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit@v1.2.0",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
			wantStderr: "v1.2.0",
		},
		{
			name:  "remote install by skill path skips full discovery",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubSkillByPath(reg, "monalisa", "skills-repo", "abc123", "skills/git-commit", "git-commit", "treeSHA")
				// DiscoverSkillByPath: tree + blob (for fetchDescription)
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
				// installer.Install: tree + blob (again, for writing files)
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "skills/git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install with URL repo argument",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "https://github.com/monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install all with collisions errors",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				// Two skills with the same install name: skills/xlsx-pro and root xlsx-pro
				treeJSON := `{"path": "skills/xlsx-pro", "type": "tree", "sha": "tree0"}, ` +
					`{"path": "skills/xlsx-pro/SKILL.md", "type": "blob", "sha": "blob0"}, ` +
					`{"path": "xlsx-pro", "type": "tree", "sha": "tree1"}, ` +
					`{"path": "xlsx-pro/SKILL.md", "type": "blob", "sha": "blob1"}`
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123", treeJSON)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					MultiSelectWithSearchFunc: func(_, _ string, _, _ []string, _ func(string) prompter.MultiSelectSearchResult) ([]string, error) {
						return []string{allSkillsKey}, nil
					},
				}
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:     pm,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantErr: "conflicting names",
		},
		{
			name:  "remote install all with namespaced skills avoids collisions",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				treeJSON := `{"path": "skills/alice", "type": "tree", "sha": "nsA"}, ` +
					`{"path": "skills/alice/xlsx-pro", "type": "tree", "sha": "treeA"}, ` +
					`{"path": "skills/alice/xlsx-pro/SKILL.md", "type": "blob", "sha": "blobA"}, ` +
					`{"path": "skills/bob", "type": "tree", "sha": "nsB"}, ` +
					`{"path": "skills/bob/xlsx-pro", "type": "tree", "sha": "treeB"}, ` +
					`{"path": "skills/bob/xlsx-pro/SKILL.md", "type": "blob", "sha": "blobB"}`
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123", treeJSON)
				// Extra blob stubs consumed by FetchDescriptionsConcurrent during interactive selection.
				contentA := base64.StdEncoding.EncodeToString([]byte("---\nname: xlsx-pro\ndescription: Alice\n---\n# A\n"))
				contentB := base64.StdEncoding.EncodeToString([]byte("---\nname: xlsx-pro\ndescription: Bob\n---\n# B\n"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/git/blobs/blobA"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "blobA", "content": %q, "encoding": "base64"}`, contentA)))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/git/blobs/blobB"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "blobB", "content": %q, "encoding": "base64"}`, contentB)))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeA", "blobA",
					"---\nname: xlsx-pro\ndescription: Alice\n---\n# A\n")
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeB", "blobB",
					"---\nname: xlsx-pro\ndescription: Bob\n---\n# B\n")
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					MultiSelectWithSearchFunc: func(_, _ string, _, _ []string, _ func(string) prompter.MultiSelectSearchResult) ([]string, error) {
						return []string{allSkillsKey}, nil
					},
				}
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:     pm,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed",
		},
		{
			name:  "remote install friendlyDir shows tilde for home paths",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "user",
					ScopeChanged: true,
				}
			},
			wantStdout: "~",
		},
		{
			name:  "interactive skill selection via prompt",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				// 31 skills to exercise maxSearchResults cap + one without description
				var treeEntries []string
				for i := range 31 {
					name := fmt.Sprintf("skill-%02d", i)
					treeEntries = append(treeEntries,
						fmt.Sprintf(`{"path": "skills/%s", "type": "tree", "sha": "tree-%s"}`, name, name),
						fmt.Sprintf(`{"path": "skills/%s/SKILL.md", "type": "blob", "sha": "blob-%s"}`, name, name))
				}
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					strings.Join(treeEntries, ", "))
				// Blob stubs for FetchDescriptionsConcurrent (one per skill)
				for i := range 31 {
					name := fmt.Sprintf("skill-%02d", i)
					blobSHA := fmt.Sprintf("blob-%s", name)
					var content string
					if i == 0 {
						// First skill has no description (exercises else branch in label building)
						content = fmt.Sprintf("---\nname: %s\n---\n# Skill\n", name)
					} else {
						content = fmt.Sprintf("---\nname: %s\ndescription: Does %s things\n---\n# Skill\n", name, name)
					}
					encoded := base64.StdEncoding.EncodeToString([]byte(content))
					reg.Register(
						httpmock.REST("GET", fmt.Sprintf("repos/monalisa/octocat-skills/git/blobs/%s", blobSHA)),
						httpmock.StringResponse(fmt.Sprintf(`{"sha": %q, "content": %q, "encoding": "base64"}`, blobSHA, encoded)))
				}
				// Install stubs for the selected skill (skill-01)
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-skill-01", "blob-skill-01",
					"---\nname: skill-01\ndescription: Does skill-01 things\n---\n# Skill\n")
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					MultiSelectWithSearchFunc: func(prompt, searchPrompt string, defaults, persistentOptions []string, searchFunc func(string) prompter.MultiSelectSearchResult) ([]string, error) {
						// Exercise searchFunc: empty query hits maxSearchResults cap (31 > 30)
						all := searchFunc("")
						if all.MoreResults == 0 {
							return nil, fmt.Errorf("expected MoreResults > 0 for 31 skills")
						}
						// Non-empty query filters down
						filtered := searchFunc("skill-01")
						if len(filtered.Keys) == 0 {
							return nil, fmt.Errorf("search returned no results")
						}
						return []string{filtered.Keys[0]}, nil
					},
					SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
						return 0, nil
					},
				}
				return &InstallOptions{
					IO:          ios,
					HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:    pm,
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "monalisa/octocat-skills",
					Agent:       "github-copilot",
					Force:       true,
				}
			},
			wantStdout: "Installed skill-01",
		},
		{
			name:  "interactive scope prompt",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
						return 0, nil
					},
				}
				return &InstallOptions{
					IO:          ios,
					HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:    pm,
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "monalisa/octocat-skills",
					SkillName:   "git-commit",
					Agent:       "github-copilot",
					Force:       true,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "interactive overwrite confirmation declined",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				destDir := t.TempDir()
				writeLocalTestSkill(t, destDir, "git-commit", gitCommitContent)
				pm := &prompter.PrompterMock{
					ConfirmFunc: func(prompt string, defaultValue bool) (bool, error) {
						return false, nil
					},
				}
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:     pm,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/octocat-skills",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          destDir,
				}
			},
			wantStderr: "No skills to install",
		},
		{
			name:  "interactive host selection via MultiSelect",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:         ios,
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:  &git.Client{RepoDir: t.TempDir()},
					Prompter: &prompter.PrompterMock{
						MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
							return []int{0}, nil // select first agent
						},
						SelectFunc: func(prompt string, defaultValue string, options []string) (int, error) {
							return 0, nil // project scope
						},
					},
					SkillSource: "monalisa/octocat-skills",
					SkillName:   "git-commit",
					Force:       true,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "scope prompt uses Remotes for repo name",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
						return 0, nil // project scope
					},
				}
				return &InstallOptions{
					IO:         ios,
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:   pm,
					GitClient:  &git.Client{RepoDir: t.TempDir()},
					Remotes: func() (context.Remotes, error) {
						return context.Remotes{
							{Remote: &git.Remote{Name: "origin"}, Repo: ghrepo.New("monalisa", "octocat-skills")},
						}, nil
					},
					SkillSource: "monalisa/octocat-skills",
					SkillName:   "git-commit",
					Agent:       "github-copilot",
					Force:       true,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "interactive overwrite shows source info",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				destDir := t.TempDir()
				existingContent := heredoc.Doc(`
					---
					name: git-commit
					description: Writes commits
					metadata:
					  github-repo: https://github.com/someowner/somerepo
					  github-ref: v0.5.0
					---
					# Git Commit
				`)
				writeLocalTestSkill(t, destDir, "git-commit", existingContent)
				pm := &prompter.PrompterMock{
					ConfirmFunc: func(prompt string, defaultValue bool) (bool, error) {
						assert.Contains(t, prompt, "someowner/somerepo@v0.5.0")
						return true, nil
					},
				}
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:     pm,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/octocat-skills",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          destDir,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "unsupported host returns error",
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:          ios,
					HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:    &prompter.PrompterMock{},
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "acme.ghes.com/monalisa/octocat-skills",
					SkillName:   "git-commit",
				}
			},
			wantErr: "supports only github.com",
		},
		{
			name:  "select all skills in interactive prompt",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				// Blob stub for FetchDescriptionsConcurrent
				encoded := base64.StdEncoding.EncodeToString([]byte(gitCommitContent))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blob-gc"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "blob-gc", "content": %q, "encoding": "base64"}`, encoded)))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					MultiSelectWithSearchFunc: func(prompt, searchPrompt string, defaults, persistentOptions []string, searchFunc func(string) prompter.MultiSelectSearchResult) ([]string, error) {
						return []string{"(all skills)"}, nil
					},
					SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
						return 0, nil
					},
				}
				return &InstallOptions{
					IO:          ios,
					HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:    pm,
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "monalisa/octocat-skills",
					Agent:       "github-copilot",
					Force:       true,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "interactive repo prompt via Input",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					InputFunc: func(prompt, defaultValue string) (string, error) {
						return "monalisa/octocat-skills", nil
					},
					SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
						return 0, nil
					},
				}
				return &InstallOptions{
					IO:         ios,
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:   pm,
					GitClient:  &git.Client{RepoDir: t.TempDir()},
					SkillName:  "git-commit",
					Agent:      "github-copilot",
					Force:      true,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "interactive scope prompt selects user scope",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
						return 1, nil // user scope
					},
				}
				return &InstallOptions{
					IO:          ios,
					HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:    pm,
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "monalisa/octocat-skills",
					SkillName:   "git-commit",
					Agent:       "github-copilot",
					Force:       true,
				}
			},
			wantStdout: "~",
		},
		{
			name:  "interactive overwrite without metadata shows plain prompt",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				destDir := t.TempDir()
				// Existing skill without github metadata in frontmatter
				writeLocalTestSkill(t, destDir, "git-commit", heredoc.Doc(`
					---
					name: git-commit
					description: No metadata
					---
					# Git Commit
				`))
				pm := &prompter.PrompterMock{
					ConfirmFunc: func(prompt string, defaultValue bool) (bool, error) {
						assert.Contains(t, prompt, "already exists")
						assert.NotContains(t, prompt, "installed from")
						return true, nil
					},
				}
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:     pm,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/octocat-skills",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          destDir,
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "remote install single exact match by name",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				treeJSON := `{"path": "skills/alice", "type": "tree", "sha": "nsA"}, ` +
					`{"path": "skills/alice/xlsx-pro", "type": "tree", "sha": "treeA"}, ` +
					`{"path": "skills/alice/xlsx-pro/SKILL.md", "type": "blob", "sha": "blobA"}, ` +
					`{"path": "skills/git-commit", "type": "tree", "sha": "treeGC"}, ` +
					`{"path": "skills/git-commit/SKILL.md", "type": "blob", "sha": "blobGC"}`
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123", treeJSON)
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeGC", "blobGC", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "multi-host install outputs per-host headers",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
					singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
				// Two install rounds (one per host)
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
				stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
						return []int{0, 1}, nil // select two agents
					},
					SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
						return 0, nil // project scope
					},
				}
				return &InstallOptions{
					IO:          ios,
					HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					Prompter:    pm,
					GitClient:   &git.Client{RepoDir: t.TempDir()},
					SkillSource: "monalisa/octocat-skills",
					SkillName:   "git-commit",
					Force:       true,
				}
			},
			wantStdout: "Installed git-commit",
			wantStderr: "Installing to",
		},
		{
			name:  "hidden-dir skills excluded without --allow-hidden-dirs",
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					hiddenDirSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
				}
			},
			wantErr: "no standard skills found, but 1 skill(s) exist in hidden directories",
		},
		{
			name:  "hidden-dir skills included with --allow-hidden-dirs",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					hiddenDirSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:              ios,
					HttpClient:      func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:       &git.Client{RepoDir: t.TempDir()},
					SkillSource:     "monalisa/skills-repo",
					SkillName:       "git-commit",
					Agent:           "github-copilot",
					Scope:           "project",
					ScopeChanged:    true,
					Dir:             t.TempDir(),
					AllowHiddenDirs: true,
				}
			},
			wantStdout: "Installed git-commit",
			wantStderr: "Skills in hidden directories",
		},
		{
			name:  "mixed tree without --allow-hidden-dirs returns only standard",
			isTTY: true,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA")+", "+
						hiddenDirSkillTreeJSON("hidden-skill", "treeSHA2", "blobSHA2"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA", "blobSHA", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:    &git.Client{RepoDir: t.TempDir()},
					SkillSource:  "monalisa/skills-repo",
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          t.TempDir(),
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "mixed tree with --allow-hidden-dirs returns all",
			isTTY: false,
			stubs: func(reg *httpmock.Registry) {
				stubResolveVersion(reg, "monalisa", "skills-repo", "v1.0.0", "abc123")
				stubDiscoverTree(reg, "monalisa", "skills-repo", "abc123",
					singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA")+", "+
						hiddenDirSkillTreeJSON("hidden-skill", "treeSHA2", "blobSHA2"))
				stubInstallFiles(reg, "monalisa", "skills-repo", "treeSHA2", "blobSHA2", gitCommitContent)
			},
			opts: func(ios *iostreams.IOStreams, reg *httpmock.Registry) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:              ios,
					HttpClient:      func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					GitClient:       &git.Client{RepoDir: t.TempDir()},
					SkillSource:     "monalisa/skills-repo",
					SkillName:       "hidden-skill",
					Agent:           "github-copilot",
					Scope:           "project",
					ScopeChanged:    true,
					Dir:             t.TempDir(),
					AllowHiddenDirs: true,
				}
			},
			wantStdout: "Installed hidden-skill",
			wantStderr: "Skills in hidden directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.stubs != nil {
				tt.stubs(reg)
			}
			if tt.setup != nil {
				tt.setup(t)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)
			opts := tt.opts(ios, reg)
			if opts.Telemetry == nil {
				opts.Telemetry = &telemetry.NoOpService{}
			}

			err := installRun(opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.wantStdout != "" {
				assert.Contains(t, stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" {
				assert.Contains(t, stderr.String(), tt.wantStderr)
			}
			if tt.verify != nil {
				tt.verify(t)
			}
		})
	}
}

func TestInstallProgress(t *testing.T) {
	ios, _, _, _ := iostreams.Test()

	assert.Nil(t, installProgress(ios, 0))
	assert.NotNil(t, installProgress(ios, 1))
	assert.NotNil(t, installProgress(ios, 2))
}

func TestInstallRun_DeduplicatesSharedProjectDirAcrossHosts(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
	stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
		singleSkillTreeJSON("git-commit", "tree-gc", "blob-gc"))
	stubInstallFiles(reg, "monalisa", "octocat-skills", "tree-gc", "blob-gc", gitCommitContent)

	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)
	ios.SetStderrTTY(true)

	pm := &prompter.PrompterMock{
		MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
			// Select two agents that share the .agents/skills project dir
			// (GitHub Copilot and Cursor) to exercise deduplication.
			var indices []int
			for i, label := range options {
				if label == "GitHub Copilot" || label == "Cursor" {
					indices = append(indices, i)
				}
			}
			return indices, nil
		},
		SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
			return 0, nil // project scope
		},
	}

	err := installRun(&InstallOptions{
		IO:          ios,
		HttpClient:  func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		Prompter:    pm,
		GitClient:   &git.Client{RepoDir: t.TempDir()},
		SkillSource: "monalisa/octocat-skills",
		SkillName:   "git-commit",
		Force:       true,
		Telemetry:   &telemetry.NoOpService{},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(stdout.String(), "Installed git-commit"))
	assert.NotContains(t, stderr.String(), "Installing to")
}

func TestRunLocalInstall(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		setup      func(t *testing.T, sourceDir, targetDir string)
		opts       func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions
		verify     func(t *testing.T, targetDir string)
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:  "installs skill with local-path metadata",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "git-commit",
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			verify: func(t *testing.T, targetDir string) {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(targetDir, "git-commit", "SKILL.md"))
				require.NoError(t, err)
				assert.Contains(t, string(data), "local-path")
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "single skill directory (SKILL.md at root)",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				content := heredoc.Doc(`
					---
					name: direct-skill
					description: Direct
					---
					# Direct
				`)
				require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "SKILL.md"), []byte(content), 0o644))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "direct-skill",
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStdout: "Installed direct-skill",
		},
		{
			name:  "namespaced skills install to separate directories",
			isTTY: true,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				for _, ns := range []string{"alice", "bob"} {
					writeLocalTestSkill(t, sourceDir, filepath.Join("skills", ns, "xlsx-pro"),
						fmt.Sprintf("---\nname: xlsx-pro\ndescription: %s xlsx-pro\n---\n# Test\n", ns))
				}
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					MultiSelectWithSearchFunc: func(_, _ string, _, _ []string, _ func(string) prompter.MultiSelectSearchResult) ([]string, error) {
						return []string{allSkillsKey}, nil
					},
				}
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					Prompter:     pm,
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			verify: func(t *testing.T, targetDir string) {
				t.Helper()
				_, err := os.Stat(filepath.Join(targetDir, "alice", "xlsx-pro", "SKILL.md"))
				assert.NoError(t, err, "alice/xlsx-pro should be installed")
				_, err = os.Stat(filepath.Join(targetDir, "bob", "xlsx-pro", "SKILL.md"))
				assert.NoError(t, err, "bob/xlsx-pro should be installed")
			},
			wantStdout: "Installed alice/xlsx-pro",
		},
		{
			name:  "local install with --force overwrites namespaced skill",
			isTTY: true,
			setup: func(t *testing.T, sourceDir, targetDir string) {
				t.Helper()
				for _, ns := range []string{"alice", "bob"} {
					writeLocalTestSkill(t, sourceDir, filepath.Join("skills", ns, "xlsx-pro"),
						fmt.Sprintf("---\nname: xlsx-pro\ndescription: %s xlsx-pro\n---\n# Test\n", ns))
				}
				require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "alice", "xlsx-pro"), 0o755))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				pm := &prompter.PrompterMock{
					MultiSelectWithSearchFunc: func(_, _ string, _, _ []string, _ func(string) prompter.MultiSelectSearchResult) ([]string, error) {
						return []string{allSkillsKey}, nil
					},
				}
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					Prompter:     pm,
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStdout: "Installed",
		},
		{
			name:  "local install existing skill without force non-interactive errors",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, targetDir string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
				require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "git-commit"), 0o755))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "git-commit",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantErr: "already installed",
		},
		{
			name:  "local install with no skills found errors",
			isTTY: false,
			setup: func(_ *testing.T, _, _ string) {},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "anything",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantErr: "no skills found",
		},
		{
			name:  "local install outputs review hint",
			isTTY: true,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "git-commit",
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStderr: "Review the installed files before use",
		},
		{
			name:  "local install with --agent claude-code",
			isTTY: true,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "git-commit",
					Force:        true,
					Agent:        "claude-code",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "local install by skill name selects one",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "code-review"), heredoc.Doc(`
					---
					name: code-review
					description: Reviews code
					---
					# Code Review
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "git-commit",
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "local install outputs file tree for TTY",
			isTTY: true,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				skillDir := filepath.Join(sourceDir, "skills", "git-commit")
				require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
					[]byte("---\nname: git-commit\ndescription: Commits\n---\n# A\n"), 0o644))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"),
					[]byte("#!/bin/bash"), 0o644))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "git-commit",
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStderr: "SKILL.md",
		},
		{
			name:  "local path with tilde expansion",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				t.Setenv("HOME", sourceDir)
				t.Setenv("USERPROFILE", sourceDir)
				return &InstallOptions{
					IO:           ios,
					SkillSource:  "~/",
					localPath:    "~/",
					SkillName:    "git-commit",
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "local path with bare tilde expansion",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				t.Setenv("HOME", sourceDir)
				t.Setenv("USERPROFILE", sourceDir)
				return &InstallOptions{
					IO:           ios,
					SkillSource:  "~",
					localPath:    "~",
					SkillName:    "git-commit",
					Force:        true,
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStdout: "Installed git-commit",
		},
		{
			name:  "local skill not found by name",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join("skills", "git-commit"), heredoc.Doc(`
					---
					name: git-commit
					description: A local skill
					---
					# Git Commit
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "nonexistent-skill",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantErr: "not found in local directory",
		},
		{
			name:  "local hidden-dir skills excluded without --allow-hidden-dirs",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join(".claude", "skills", "code-review"), heredoc.Doc(`
					---
					name: code-review
					description: Reviews code
					---
					# Code Review
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:           ios,
					SkillSource:  sourceDir,
					localPath:    sourceDir,
					SkillName:    "code-review",
					Agent:        "github-copilot",
					Scope:        "project",
					ScopeChanged: true,
					Dir:          targetDir,
					GitClient:    &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantErr: "no standard skills found, but 1 skill(s) exist in hidden directories",
		},
		{
			name:  "local hidden-dir skills included with --allow-hidden-dirs",
			isTTY: false,
			setup: func(t *testing.T, sourceDir, _ string) {
				t.Helper()
				writeLocalTestSkill(t, sourceDir, filepath.Join(".claude", "skills", "code-review"), heredoc.Doc(`
					---
					name: code-review
					description: Reviews code
					---
					# Code Review
				`))
			},
			opts: func(ios *iostreams.IOStreams, sourceDir, targetDir string) *InstallOptions {
				t.Helper()
				return &InstallOptions{
					IO:              ios,
					SkillSource:     sourceDir,
					localPath:       sourceDir,
					SkillName:       "code-review",
					Force:           true,
					Agent:           "github-copilot",
					Scope:           "project",
					ScopeChanged:    true,
					Dir:             targetDir,
					AllowHiddenDirs: true,
					GitClient:       &git.Client{RepoDir: t.TempDir()},
				}
			},
			wantStdout: "Installed code-review",
			wantStderr: "Skills in hidden directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)

			sourceDir := t.TempDir()
			targetDir := t.TempDir()

			if tt.setup != nil {
				tt.setup(t, sourceDir, targetDir)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)
			opts := tt.opts(ios, sourceDir, targetDir)

			err := installRun(opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.wantStdout != "" {
				assert.Contains(t, stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" {
				assert.Contains(t, stderr.String(), tt.wantStderr)
			}
			if tt.verify != nil {
				tt.verify(t, targetDir)
			}
		})
	}
}

func Test_printReviewHint(t *testing.T) {
	tests := []struct {
		name       string
		repo       string
		sha        string
		skillNames []string
		wantOutput string
	}{
		{
			name:       "remote install with SHA includes SHA in preview command",
			repo:       "owner/repo",
			sha:        "abc123def456",
			skillNames: []string{"my-skill"},
			wantOutput: "gh skill preview owner/repo my-skill@abc123def456",
		},
		{
			name:       "remote install without SHA omits SHA from preview command",
			repo:       "owner/repo",
			sha:        "",
			skillNames: []string{"my-skill"},
			wantOutput: "gh skill preview owner/repo my-skill\n",
		},
		{
			name:       "multiple skills with SHA",
			repo:       "owner/repo",
			sha:        "deadbeef",
			skillNames: []string{"skill-a", "skill-b"},
			wantOutput: "skill-a@deadbeef",
		},
		{
			name:       "local install shows generic message",
			repo:       "",
			sha:        "",
			skillNames: []string{"my-skill"},
			wantOutput: "Review the installed files before use",
		},
		{
			name:       "no skills produces no output",
			repo:       "owner/repo",
			sha:        "abc123",
			skillNames: []string{},
			wantOutput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			cs := ios.ColorScheme()
			var buf strings.Builder
			printReviewHint(&buf, cs, tt.repo, tt.sha, tt.skillNames)
			if tt.wantOutput == "" {
				assert.Empty(t, buf.String())
			} else {
				assert.Contains(t, buf.String(), tt.wantOutput)
			}
		})
	}
}

func Test_printHostHints(t *testing.T) {
	kiro := &registry.AgentHost{ID: "kiro-cli", Name: "Kiro CLI", ProjectDir: ".kiro/skills", UserDir: ".kiro/skills"}
	copilot := &registry.AgentHost{ID: "copilot-cli", Name: "GitHub Copilot CLI", ProjectDir: ".github/skills"}

	tests := []struct {
		name       string
		hosts      []*registry.AgentHost
		installed  []string
		installDir string
		gitRoot    string
		wantSub    []string
		wantNot    []string
	}{
		{
			name:       "no installs produces no output",
			hosts:      []*registry.AgentHost{kiro},
			installed:  nil,
			installDir: "/repo/.kiro/skills",
			gitRoot:    "/repo",
			wantNot:    []string{"Kiro CLI"},
		},
		{
			name:       "non-kiro host produces no output",
			hosts:      []*registry.AgentHost{copilot},
			installed:  []string{"s1"},
			installDir: "/repo/.github/skills",
			gitRoot:    "/repo",
			wantNot:    []string{"Kiro CLI"},
		},
		{
			name:       "kiro project scope uses relative path",
			hosts:      []*registry.AgentHost{kiro},
			installed:  []string{"s1"},
			installDir: filepath.Join("/repo", ".kiro", "skills"),
			gitRoot:    "/repo",
			wantSub:    []string{"Kiro CLI", `"skill://.kiro/skills/**/SKILL.md"`},
		},
		{
			name:       "kiro user scope uses absolute install dir",
			hosts:      []*registry.AgentHost{kiro},
			installed:  []string{"s1"},
			installDir: "/home/user/.kiro/skills",
			gitRoot:    "/repo",
			wantSub:    []string{`"skill:///home/user/.kiro/skills/**/SKILL.md"`},
			wantNot:    []string{`skill://.kiro/skills`},
		},
		{
			name:       "kiro custom dir outside git root uses absolute path",
			hosts:      []*registry.AgentHost{kiro},
			installed:  []string{"s1"},
			installDir: "/tmp/my-skills",
			gitRoot:    "/repo",
			wantSub:    []string{`"skill:///tmp/my-skills/**/SKILL.md"`},
		},
		{
			name:       "kiro without git root falls back to install dir",
			hosts:      []*registry.AgentHost{kiro},
			installed:  []string{"s1"},
			installDir: "/home/user/.kiro/skills",
			gitRoot:    "",
			wantSub:    []string{`"skill:///home/user/.kiro/skills/**/SKILL.md"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			cs := ios.ColorScheme()
			var buf strings.Builder
			printHostHints(&buf, cs, tt.hosts, tt.installed, tt.installDir, tt.gitRoot)
			got := buf.String()
			for _, s := range tt.wantSub {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.wantNot {
				assert.NotContains(t, got, s)
			}
		})
	}
}

func Test_printPreInstallDisclaimer(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cs := ios.ColorScheme()
	var buf strings.Builder
	printPreInstallDisclaimer(&buf, cs)
	output := buf.String()
	assert.Contains(t, output, "not verified by GitHub")
	assert.Contains(t, output, "prompt")
	assert.Contains(t, output, "malicious")
}

func Test_selectSkillsWithSelector_noDisclaimer(t *testing.T) {
	ios, _, _, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

	skills := []discovery.Skill{
		{Name: "git-commit", Convention: "skills", Path: "skills/git-commit/SKILL.md"},
	}

	pm := &prompter.PrompterMock{
		MultiSelectWithSearchFunc: func(_, _ string, _, _ []string, _ func(string) prompter.MultiSelectSearchResult) ([]string, error) {
			return []string{"git-commit"}, nil
		},
	}

	opts := &InstallOptions{
		IO:       ios,
		Prompter: pm,
	}

	_, err := selectSkillsWithSelector(opts, skills, true, skillSelector{
		matchByName: matchSkillByName,
		sourceHint:  "owner/repo",
	})
	require.NoError(t, err)
	assert.NotContains(t, stderr.String(), "not verified by GitHub")
}

func TestInstallRun_TelemetryVisibility(t *testing.T) {
	tests := []struct {
		name           string
		visibility     string
		visibilityErr  bool
		wantSkillNames string
	}{
		{
			name:           "public repo includes skill names",
			visibility:     "public",
			wantSkillNames: "git-commit",
		},
		{
			name:       "private repo excludes skill names",
			visibility: "private",
		},
		{
			name:       "internal repo excludes skill names",
			visibility: "internal",
		},
		{
			name:          "API error omits visibility and skill names",
			visibilityErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
			stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123",
				singleSkillTreeJSON("git-commit", "treeSHA", "blobSHA"))
			stubInstallFiles(reg, "monalisa", "octocat-skills", "treeSHA", "blobSHA", gitCommitContent)
			if tt.visibilityErr {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.StatusStringResponse(500, "server error"),
				)
			} else {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{
						"visibility": tt.visibility,
					}),
				)
			}

			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)

			recorder := &telemetry.EventRecorderSpy{}

			err := installRun(&InstallOptions{
				IO:           ios,
				HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
				GitClient:    &git.Client{RepoDir: t.TempDir()},
				Prompter:     &prompter.PrompterMock{},
				SkillSource:  "monalisa/octocat-skills",
				SkillName:    "git-commit",
				Agent:        "github-copilot",
				Scope:        "project",
				ScopeChanged: true,
				Dir:          t.TempDir(),
				Force:        true,
				Telemetry:    recorder,
			})
			require.NoError(t, err)

			require.Len(t, recorder.Events, 1)
			event := recorder.Events[0]
			assert.Equal(t, "skill_install", event.Type)
			assert.NotEmpty(t, event.Dimensions["agent_hosts"], "agent_hosts should always be present")

			// skill_host_type is always recorded (categorized, no raw hostname for enterprise/tenancy).
			assert.Equal(t, "github.com", event.Dimensions["skill_host_type"])

			if tt.visibilityErr {
				assert.Equal(t, "unknown", event.Dimensions["repo_visibility"],
					"visibility fetch errors should emit repo_visibility=\"unknown\" so the fallback is distinguishable from a successful fetch")
			} else {
				assert.Equal(t, tt.visibility, event.Dimensions["repo_visibility"])
			}

			// Owner, repo, and skill names are only included when the repo
			// is public; for private/internal/unknown they are omitted to
			// avoid leaking identifiers of non-public repositories.
			if tt.wantSkillNames != "" {
				assert.Equal(t, "monalisa", event.Dimensions["skill_owner"])
				assert.Equal(t, "octocat-skills", event.Dimensions["skill_repo"])
				assert.Equal(t, tt.wantSkillNames, event.Dimensions["skill_names"])
			} else {
				assert.Empty(t, event.Dimensions["skill_owner"])
				assert.Empty(t, event.Dimensions["skill_repo"])
				assert.Empty(t, event.Dimensions["skill_names"])
			}
		})
	}
}

func TestInstallRun_TelemetryMultipleSkills(t *testing.T) {
	codeReviewContent := heredoc.Doc(`
		---
		name: code-review
		description: Reviews code
		---
		# Code Review
	`)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	stubResolveVersion(reg, "monalisa", "octocat-skills", "v1.0.0", "abc123")
	treeJSON := `{"path": "skills/git-commit", "type": "tree", "sha": "treeGC"}, ` +
		`{"path": "skills/git-commit/SKILL.md", "type": "blob", "sha": "blobGC"}, ` +
		`{"path": "skills/code-review", "type": "tree", "sha": "treeCR"}, ` +
		`{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "blobCR"}`
	stubDiscoverTree(reg, "monalisa", "octocat-skills", "abc123", treeJSON)

	// Blob stubs for FetchDescriptionsConcurrent during interactive selection
	encGC := base64.StdEncoding.EncodeToString([]byte(gitCommitContent))
	encCR := base64.StdEncoding.EncodeToString([]byte(codeReviewContent))
	reg.Register(
		httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blobGC"),
		httpmock.StringResponse(fmt.Sprintf(`{"sha": "blobGC", "content": %q, "encoding": "base64"}`, encGC)))
	reg.Register(
		httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blobCR"),
		httpmock.StringResponse(fmt.Sprintf(`{"sha": "blobCR", "content": %q, "encoding": "base64"}`, encCR)))

	stubInstallFiles(reg, "monalisa", "octocat-skills", "treeGC", "blobGC", gitCommitContent)
	stubInstallFiles(reg, "monalisa", "octocat-skills", "treeCR", "blobCR", codeReviewContent)

	reg.Register(
		httpmock.REST("GET", "repos/monalisa/octocat-skills"),
		httpmock.JSONResponse(map[string]interface{}{
			"visibility": "public",
		}),
	)

	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)

	pm := &prompter.PrompterMock{
		MultiSelectWithSearchFunc: func(_, _ string, _, _ []string, _ func(string) prompter.MultiSelectSearchResult) ([]string, error) {
			return []string{allSkillsKey}, nil
		},
	}

	recorder := &telemetry.EventRecorderSpy{}

	err := installRun(&InstallOptions{
		IO:           ios,
		HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		GitClient:    &git.Client{RepoDir: t.TempDir()},
		Prompter:     pm,
		SkillSource:  "monalisa/octocat-skills",
		Agent:        "github-copilot",
		Scope:        "project",
		ScopeChanged: true,
		Dir:          t.TempDir(),
		Telemetry:    recorder,
	})
	require.NoError(t, err)

	require.Len(t, recorder.Events, 1)
	event := recorder.Events[0]
	assert.Equal(t, "skill_install", event.Type)
	assert.Equal(t, "public", event.Dimensions["repo_visibility"])

	// Verify comma-separated skill names (alphabetical order from DiscoverSkills)
	names := strings.Split(event.Dimensions["skill_names"], ",")
	assert.Len(t, names, 2)
	assert.Contains(t, names, "code-review")
	assert.Contains(t, names, "git-commit")
}
