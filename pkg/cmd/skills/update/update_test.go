package update

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/skills/registry"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdUpdate_Help(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Prompter:  &prompter.PrompterMock{},
		GitClient: &git.Client{},
	}

	cmd := NewCmdUpdate(f, func(opts *UpdateOptions) error {
		return nil
	})

	assert.Equal(t, "update [<skill>...] [flags]", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
}

func TestNewCmdUpdate_Flags(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Prompter: &prompter.PrompterMock{}, GitClient: &git.Client{}}
	cmd := NewCmdUpdate(f, func(_ *UpdateOptions) error { return nil })

	flags := []string{"all", "force", "dry-run", "dir", "unpin"}
	for _, name := range flags {
		assert.NotNil(t, cmd.Flags().Lookup(name), "missing flag: --%s", name)
	}
}

func TestNewCmdUpdate_ArgsPassedToOptions(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	f := &cmdutil.Factory{IOStreams: ios, Prompter: &prompter.PrompterMock{}, GitClient: &git.Client{}}

	var gotOpts *UpdateOptions
	cmd := NewCmdUpdate(f, func(opts *UpdateOptions) error {
		gotOpts = opts
		return nil
	})

	args, _ := shlex.Split("mcp-cli git-commit --all --force")
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, []string{"mcp-cli", "git-commit"}, gotOpts.Skills)
	assert.True(t, gotOpts.All)
	assert.True(t, gotOpts.Force)
}

func TestScanInstalledSkills(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, dir string)
		verify func(t *testing.T, skills []installedSkill, err error)
	}{
		{
			name: "happy path with metadata, no metadata, and pinned skills",
			setup: func(t *testing.T, dir string) {
				t.Helper()

				// Skill with full metadata
				skillDir := filepath.Join(dir, "git-commit")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				content := heredoc.Doc(`
					---
					name: git-commit
					description: Git commit helper
					metadata:
					  github-repo: https://github.com/monalisa/awesome-copilot
					  github-tree-sha: abc123
					  github-path: skills/git-commit
					---
					Body content
				`)
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

				// Skill without metadata
				noMetaDir := filepath.Join(dir, "unknown-skill")
				require.NoError(t, os.MkdirAll(noMetaDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(noMetaDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: unknown-skill
					---
					No metadata here
				`)), 0o644))

				// Pinned skill
				pinnedDir := filepath.Join(dir, "pinned-skill")
				require.NoError(t, os.MkdirAll(pinnedDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(pinnedDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: pinned-skill
					metadata:
					  github-repo: https://github.com/octocat/hubot-skills
					  github-tree-sha: def456
					  github-pinned: v1.0.0
					---
					Pinned content
				`)), 0o644))
			},
			verify: func(t *testing.T, skills []installedSkill, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Len(t, skills, 3)

				byName := make(map[string]installedSkill)
				for _, s := range skills {
					byName[s.name] = s
				}

				gc := byName["git-commit"]
				assert.Equal(t, "monalisa", gc.owner)
				assert.Equal(t, "awesome-copilot", gc.repo)
				assert.Equal(t, "github.com", gc.repoHost)
				assert.Equal(t, "abc123", gc.treeSHA)
				assert.Equal(t, "skills/git-commit", gc.sourcePath)
				assert.Empty(t, gc.pinned)

				us := byName["unknown-skill"]
				assert.Empty(t, us.owner)
				assert.Empty(t, us.repo)

				ps := byName["pinned-skill"]
				assert.Equal(t, "github.com", ps.repoHost)
				assert.Equal(t, "v1.0.0", ps.pinned)
			},
		},
		{
			name: "unsupported host metadata returns error",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "enterprise-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: enterprise-skill
					metadata:
					  github-repo: https://acme.ghes.com/monalisa/octocat-skills
					  github-tree-sha: abc123
					---
					body
				`)), 0o644))
			},
			verify: func(t *testing.T, skills []installedSkill, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, skills, 1)
				require.Error(t, skills[0].metadataErr)
				assert.Contains(t, skills[0].metadataErr.Error(), "does not currently support GitHub Enterprise Server")
			},
		},
		{
			name: "non-existent directory returns nil",
			// no setup needed; dir does not exist
			verify: func(t *testing.T, skills []installedSkill, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Nil(t, skills)
			},
		},
		{
			name: "corrupted YAML is skipped gracefully",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "corrupt")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					not: valid: yaml: [broken
					---
					body
				`)), 0o644))
			},
			verify: func(t *testing.T, skills []installedSkill, err error) {
				t.Helper()
				require.NoError(t, err)
				require.Len(t, skills, 1)
				assert.Equal(t, "corrupt", skills[0].name)
				assert.ErrorContains(t, skills[0].metadataErr, "invalid SKILL.md")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For the non-existent directory case, pass a path that doesn't exist
			dir := filepath.Join(t.TempDir(), "skills")
			if tt.setup != nil {
				require.NoError(t, os.MkdirAll(dir, 0o755))
				tt.setup(t, dir)
			}

			skills, err := scanInstalledSkills(dir, nil, "")
			tt.verify(t, skills, err)
		})
	}
}

func TestPromptForSkillOrigin(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantOK     bool
		wantOwner  string
		wantRepo   string
		wantReason string
	}{
		{
			name:      "valid owner/repo",
			input:     "monalisa/awesome-copilot",
			wantOK:    true,
			wantOwner: "monalisa",
			wantRepo:  "awesome-copilot",
		},
		{
			name:   "empty input skips",
			input:  "",
			wantOK: false,
		},
		{
			name:       "invalid format returns reason",
			input:      "just-a-name",
			wantOK:     false,
			wantReason: "invalid repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := &prompter.PrompterMock{
				InputFunc: func(prompt string, defaultValue string) (string, error) {
					return tt.input, nil
				},
			}

			owner, repo, reason, ok, err := promptForSkillOrigin(pm, "test-skill")
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
			if tt.wantReason != "" {
				assert.Contains(t, reason, tt.wantReason)
			}
		})
	}
}

func TestScanAllAgentsDeduplicatesSharedProjectDirs(t *testing.T) {
	repoDir := t.TempDir()
	homeDir := t.TempDir()

	sharedSkillDir := filepath.Join(repoDir, ".agents", "skills", "git-commit")
	require.NoError(t, os.MkdirAll(sharedSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sharedSkillDir, "SKILL.md"), []byte(heredoc.Doc(`
		---
		name: git-commit
		metadata:
		  github-repo: https://github.com/monalisa/octocat-skills
		  github-tree-sha: abc123
		---
		Body
	`)), 0o644))

	claudeSkillDir := filepath.Join(repoDir, ".claude", "skills", "code-review")
	require.NoError(t, os.MkdirAll(claudeSkillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeSkillDir, "SKILL.md"), []byte(heredoc.Doc(`
		---
		name: code-review
		metadata:
		  github-repo: https://github.com/monalisa/octocat-skills
		  github-tree-sha: def456
		---
		Body
	`)), 0o644))

	skills := scanAllAgents(repoDir, homeDir)
	require.Len(t, skills, 2)

	byName := make(map[string]installedSkill)
	for _, skill := range skills {
		byName[skill.name] = skill
	}

	assert.Equal(t, registry.ScopeProject, byName["git-commit"].scope)
	assert.Equal(t, registry.ScopeProject, byName["code-review"].scope)
}

func TestUpdateRun(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir string)
		stubs      func(reg *httpmock.Registry)
		opts       func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions
		verify     func(t *testing.T, dir string)
		wantErr    string
		wantStderr string
		wantStdout string
	}{
		{
			name: "scans all agents when no --dir is set",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				t.Setenv("HOME", dir)
				t.Setenv("USERPROFILE", dir)
				skillDir := filepath.Join(dir, ".agents", "skills", "code-review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
				---
				name: code-review
				metadata:
				  github-repo: https://github.com/monalisa/octocat-skills
				  github-tree-sha: currentsha
				  github-path: skills/code-review
				---
				Installed content
			`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "commit1", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/commit1"),
					httpmock.StringResponse(`{"sha": "commit1", "tree": [{"path": "skills/code-review", "type": "tree", "sha": "currentsha"}, {"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "blob1"}, {"path": "skills", "type": "tree", "sha": "treeshaX"}], "truncated": false}`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
				}
			},
			wantStderr: "All skills are up to date.",
		},
		{
			name:  "no installed skills",
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			wantStderr: "No installed skills found.",
		},
		{
			name: "specific skill not installed",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "octocat-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: octocat-skill
					metadata:
					  github-repo: https://github.com/octocat/hubot-skills
					  github-tree-sha: abc
					---
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					Skills:    []string{"nonexistent"},
				}
			},
			wantErr: "none of the specified skills are installed",
		},
		{
			name: "pinned skills are skipped",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "pinned-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: pinned-skill
					metadata:
					  github-repo: https://github.com/octocat/hubot-skills
					  github-tree-sha: abc123
					  github-pinned: v1.0.0
					---
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter:  &prompter.PrompterMock{},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			wantStderr: "pinned",
		},
		{
			name: "no metadata skips in non-interactive mode",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "manual-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: manual-skill
					---
					No metadata
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				ios.SetStdinTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			wantStderr: "no GitHub metadata",
		},
		{
			name: "all skips no-metadata skill without prompting",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "manual-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: manual-skill
					---
					No metadata
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStdinTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter: &prompter.PrompterMock{
						InputFunc: func(prompt string, defaultValue string) (string, error) {
							return "", fmt.Errorf("unexpected prompt")
						},
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					All:       true,
				}
			},
			wantStderr: "Run `gh skill update manual-skill` interactively",
		},
		{
			name: "all up to date",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "monalisa-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: monalisa-skill
					metadata:
					  github-repo: https://github.com/monalisa/octocat-skills
					  github-tree-sha: abc123def456
					  github-path: skills/monalisa-skill
					---
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "commitsha123", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/commitsha123"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "commitsha123", "tree": [{"path": "skills/monalisa-skill/SKILL.md", "type": "blob", "sha": "blobsha1"}, {"path": "skills/monalisa-skill", "type": "tree", "sha": "abc123def456"}, {"path": "skills", "type": "tree", "sha": "treeshaX"}], "truncated": false}`)),
				)
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			wantStderr: "All skills are up to date.",
		},
		{
			name: "dry run reports available updates",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "hubot-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: hubot-skill
					metadata:
					  github-repo: https://github.com/hubot/octocat-skills
					  github-tree-sha: oldsha123
					  github-path: skills/hubot-skill
					---
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/hubot/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v2.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/hubot/octocat-skills/git/ref/tags/v2.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit456", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/hubot/octocat-skills/git/trees/newcommit456"),
					httpmock.StringResponse(`{"sha": "newcommit456", "tree": [{"path": "skills/hubot-skill/SKILL.md", "type": "blob", "sha": "blobsha2"}, {"path": "skills/hubot-skill", "type": "tree", "sha": "newsha456"}, {"path": "skills", "type": "tree", "sha": "treeshaY"}], "truncated": false}`),
				)
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter:  &prompter.PrompterMock{},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					DryRun:    true,
				}
			},
			wantStderr: "1 update(s) available:",
			wantStdout: "hubot-skill",
		},
		{
			name: "non-interactive without --all errors",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "hubot-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: hubot-skill
					metadata:
					  github-repo: https://github.com/hubot/octocat-skills
					  github-tree-sha: oldsha123
					  github-path: skills/hubot-skill
					---
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/hubot/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v2.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/hubot/octocat-skills/git/ref/tags/v2.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit456", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/hubot/octocat-skills/git/trees/newcommit456"),
					httpmock.StringResponse(`{"sha": "newcommit456", "tree": [{"path": "skills/hubot-skill/SKILL.md", "type": "blob", "sha": "blobsha2"}, {"path": "skills/hubot-skill", "type": "tree", "sha": "newsha456"}, {"path": "skills", "type": "tree", "sha": "treeshaY"}], "truncated": false}`),
				)
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				ios.SetStdinTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			wantErr: "updates available; re-run with --all to apply, or run interactively to confirm",
		},
		{
			name: "force update rewrites SKILL.md on disk",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				t.Setenv("USERPROFILE", homeDir)
				skillDir := filepath.Join(dir, "code-review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: code-review
					metadata:
					  github-repo: https://github.com/monalisa/octocat-skills
					  github-tree-sha: oldsha000
					  github-path: skills/code-review
					---
					Old content
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v3.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v3.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit789", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newcommit789"),
					httpmock.StringResponse(`{"sha": "newcommit789", "tree": [{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "newblob1"}, {"path": "skills/code-review", "type": "tree", "sha": "newsha999"}, {"path": "skills", "type": "tree", "sha": "treeshaZ"}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newsha999"),
					httpmock.StringResponse(`{"sha": "newsha999", "tree": [{"path": "SKILL.md", "type": "blob", "sha": "newblob1", "size": 20}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/newblob1"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "newblob1", "encoding": "base64", "content": "%s"}`,
						"IyBDb2RlIFJldmlldyBVcGRhdGVk")))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					All:       true,
					Force:     true,
				}
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "code-review", "SKILL.md"))
				require.NoError(t, err)
				assert.Contains(t, string(content), "github-repo: https://github.com/monalisa/octocat-skills")
				assert.NotContains(t, string(content), "Old content")
			},
			wantStdout: "Updated code-review",
		},
		{
			name: "namespaced skill with --dir updates in-place",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				t.Setenv("USERPROFILE", homeDir)
				skillDir := filepath.Join(dir, "monalisa", "code-review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: code-review
					metadata:
					  github-repo: https://github.com/monalisa/octocat-skills
					  github-tree-sha: oldsha000
					  github-path: skills/monalisa/code-review
					---
					Old namespaced content
				`)), 0o644))
				// Plant a stale file that should be cleaned during update.
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "STALE.txt"), []byte("leftover"), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v3.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v3.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit789", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newcommit789"),
					httpmock.StringResponse(`{"sha": "newcommit789", "tree": [{"path": "skills/monalisa/code-review/SKILL.md", "type": "blob", "sha": "newblob1"}, {"path": "skills/monalisa/code-review", "type": "tree", "sha": "newsha999"}, {"path": "skills/monalisa", "type": "tree", "sha": "nstresha"}, {"path": "skills", "type": "tree", "sha": "treeshaZ"}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newsha999"),
					httpmock.StringResponse(`{"sha": "newsha999", "tree": [{"path": "SKILL.md", "type": "blob", "sha": "newblob1", "size": 20}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/newblob1"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "newblob1", "encoding": "base64", "content": "%s"}`,
						"IyBOYW1lc3BhY2VkIFNraWxsIFVwZGF0ZWQ=")))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					All:       true,
					Force:     true,
				}
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				// Skill must stay in its original namespaced directory.
				content, err := os.ReadFile(filepath.Join(dir, "monalisa", "code-review", "SKILL.md"))
				require.NoError(t, err)
				assert.Contains(t, string(content), "github-repo: https://github.com/monalisa/octocat-skills")
				assert.NotContains(t, string(content), "Old namespaced content")
				// Skill must NOT have been relocated to a flat path.
				_, err = os.Stat(filepath.Join(dir, "code-review", "SKILL.md"))
				assert.True(t, os.IsNotExist(err), "skill should not be relocated to flat path")
				// Namespace directory must still exist.
				_, err = os.Stat(filepath.Join(dir, "monalisa", "code-review"))
				assert.False(t, os.IsNotExist(err), "namespaced directory must not be deleted")
				// Stale file should have been cleaned during update.
				_, err = os.Stat(filepath.Join(dir, "monalisa", "code-review", "STALE.txt"))
				assert.True(t, os.IsNotExist(err), "stale file should be removed during update")
			},
			wantStdout: "Updated monalisa/code-review",
		},
		{
			name: "install failure during update reports error and continues",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				t.Setenv("USERPROFILE", homeDir)
				skillDir := filepath.Join(dir, "code-review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: code-review
					metadata:
					  github-repo: https://github.com/monalisa/octocat-skills
					  github-tree-sha: oldsha000
					  github-path: skills/code-review
					---
					Original content
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v3.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v3.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit789", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newcommit789"),
					httpmock.StringResponse(`{"sha": "newcommit789", "tree": [{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "newblob1"}, {"path": "skills/code-review", "type": "tree", "sha": "newsha999"}, {"path": "skills", "type": "tree", "sha": "treeshaZ"}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newsha999"),
					httpmock.StringResponse(`{"sha": "newsha999", "tree": [{"path": "SKILL.md", "type": "blob", "sha": "newblob1", "size": 20}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/newblob1"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					All:       true,
				}
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "code-review", "SKILL.md"))
				require.NoError(t, err)
				assert.Contains(t, string(content), "Original content", "file should not be modified on failure")
			},
			wantStderr: "Failed to update code-review",
			wantErr:    "SilentError",
		},
		{
			name: "interactive confirm applies update",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				t.Setenv("USERPROFILE", homeDir)
				skillDir := filepath.Join(dir, "code-review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: code-review
					metadata:
					  github-repo: https://github.com/monalisa/octocat-skills
					  github-tree-sha: oldsha000
					  github-path: skills/code-review
					---
					Old content
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v3.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v3.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit789", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newcommit789"),
					httpmock.StringResponse(`{"sha": "newcommit789", "tree": [{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "newblob1"}, {"path": "skills/code-review", "type": "tree", "sha": "newsha999"}, {"path": "skills", "type": "tree", "sha": "treeshaZ"}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newsha999"),
					httpmock.StringResponse(`{"sha": "newsha999", "tree": [{"path": "SKILL.md", "type": "blob", "sha": "newblob1", "size": 20}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/newblob1"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "newblob1", "encoding": "base64", "content": "%s"}`,
						"IyBDb2RlIFJldmlldyBVcGRhdGVk")))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStdinTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, defaultVal bool) (bool, error) {
							return true, nil
						},
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "code-review", "SKILL.md"))
				require.NoError(t, err)
				assert.NotContains(t, string(content), "Old content")
			},
			wantStdout: "Updated code-review",
		},
		{
			name: "interactive confirm cancelled",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "code-review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: code-review
					metadata:
					  github-repo: https://github.com/monalisa/octocat-skills
					  github-tree-sha: oldsha000
					  github-path: skills/code-review
					---
					Old content
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v3.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v3.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit789", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newcommit789"),
					httpmock.StringResponse(`{"sha": "newcommit789", "tree": [{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "newblob1"}, {"path": "skills/code-review", "type": "tree", "sha": "newsha999"}, {"path": "skills", "type": "tree", "sha": "treeshaZ"}], "truncated": false}`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStdinTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, defaultVal bool) (bool, error) {
							return false, nil
						},
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			wantErr:    "CancelError",
			wantStderr: "Update cancelled",
		},
		{
			name: "no-metadata skill prompted interactively and skipped",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "manual-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: manual-skill
					---
					No metadata
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStdinTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter: &prompter.PrompterMock{
						InputFunc: func(prompt string, defaultValue string) (string, error) {
							return "", nil
						},
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			wantStderr: "no GitHub metadata",
		},
		{
			name: "no-metadata skill enriched via prompt then updated",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				t.Setenv("USERPROFILE", homeDir)
				skillDir := filepath.Join(dir, "manual-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: manual-skill
					---
					Old manual content
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "commit123", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/commit123"),
					httpmock.StringResponse(`{"sha": "commit123", "tree": [{"path": "skills/manual-skill/SKILL.md", "type": "blob", "sha": "blob1"}, {"path": "skills/manual-skill", "type": "tree", "sha": "newtree1"}, {"path": "skills", "type": "tree", "sha": "treeshaX"}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/newtree1"),
					httpmock.StringResponse(`{"sha": "newtree1", "tree": [{"path": "SKILL.md", "type": "blob", "sha": "blob1", "size": 20}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blob1"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "blob1", "encoding": "base64", "content": "%s"}`,
						"IyBNYW51YWwgU2tpbGwgVXBkYXRlZA==")))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStdinTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter: &prompter.PrompterMock{
						InputFunc: func(prompt string, defaultValue string) (string, error) {
							return "monalisa/octocat-skills", nil
						},
						ConfirmFunc: func(msg string, defaultVal bool) (bool, error) {
							return true, nil
						},
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
				}
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "manual-skill", "SKILL.md"))
				require.NoError(t, err)
				assert.NotContains(t, string(content), "Old manual content")
				assert.Contains(t, string(content), "github-repo: https://github.com/monalisa/octocat-skills")
			},
			wantStdout: "Updated manual-skill",
		},
		{
			name: "unpin clears pin and applies update",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				homeDir := t.TempDir()
				t.Setenv("HOME", homeDir)
				t.Setenv("USERPROFILE", homeDir)
				skillDir := filepath.Join(dir, "pinned-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: pinned-skill
					metadata:
					  github-repo: https://github.com/octocat/hubot-skills
					  github-tree-sha: oldsha000
					  github-pinned: v1.0.0
					  github-path: skills/pinned-skill
					---
					Pinned content
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v2.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/git/ref/tags/v2.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit789", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/git/trees/newcommit789"),
					httpmock.StringResponse(`{"sha": "newcommit789", "tree": [{"path": "skills/pinned-skill/SKILL.md", "type": "blob", "sha": "newblob1"}, {"path": "skills/pinned-skill", "type": "tree", "sha": "newsha999"}, {"path": "skills", "type": "tree", "sha": "treeshaZ"}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/git/trees/newsha999"),
					httpmock.StringResponse(`{"sha": "newsha999", "tree": [{"path": "SKILL.md", "type": "blob", "sha": "newblob1", "size": 20}], "truncated": false}`))
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/git/blobs/newblob1"),
					httpmock.StringResponse(fmt.Sprintf(`{"sha": "newblob1", "encoding": "base64", "content": "%s"}`,
						"IyBVbnBpbm5lZCBhbmQgVXBkYXRlZA==")))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(false)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					All:       true,
					Unpin:     true,
				}
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "pinned-skill", "SKILL.md"))
				require.NoError(t, err)
				assert.NotContains(t, string(content), "Pinned content")
				assert.NotContains(t, string(content), "github-pinned")
			},
			wantStdout: "Updated pinned-skill",
		},
		{
			name: "pinned skills still skipped without --unpin",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "pinned-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: pinned-skill
					metadata:
					  github-repo: https://github.com/octocat/hubot-skills
					  github-tree-sha: abc123
					  github-pinned: v1.0.0
					---
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter:  &prompter.PrompterMock{},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					Unpin:     false,
				}
			},
			wantStderr: "pinned",
		},
		{
			name: "unpin with dry-run reports update without modifying files",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, "pinned-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: pinned-skill
					metadata:
					  github-repo: https://github.com/octocat/hubot-skills
					  github-tree-sha: oldsha000
					  github-pinned: v1.0.0
					  github-path: skills/pinned-skill
					---
					Pinned content
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v2.0.0"}`))
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/git/ref/tags/v2.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "newcommit789", "type": "commit"}}`))
				reg.Register(
					httpmock.REST("GET", "repos/octocat/hubot-skills/git/trees/newcommit789"),
					httpmock.StringResponse(`{"sha": "newcommit789", "tree": [{"path": "skills/pinned-skill/SKILL.md", "type": "blob", "sha": "newblob1"}, {"path": "skills/pinned-skill", "type": "tree", "sha": "newsha999"}, {"path": "skills", "type": "tree", "sha": "treeshaZ"}], "truncated": false}`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *UpdateOptions {
				ios.SetStdoutTTY(true)
				ios.SetStderrTTY(true)
				return &UpdateOptions{
					IO:     ios,
					Config: func() (gh.Config, error) { return config.NewBlankConfig(), nil },
					HttpClient: func() (*http.Client, error) {
						return &http.Client{Transport: reg}, nil
					},
					Prompter:  &prompter.PrompterMock{},
					GitClient: &git.Client{RepoDir: dir},
					Dir:       dir,
					DryRun:    true,
					Unpin:     true,
				}
			},
			verify: func(t *testing.T, dir string) {
				t.Helper()
				content, err := os.ReadFile(filepath.Join(dir, "pinned-skill", "SKILL.md"))
				require.NoError(t, err)
				assert.Contains(t, string(content), "github-pinned: v1.0.0", "dry-run should not modify files")
			},
			wantStderr: "1 update(s) available:",
			wantStdout: "pinned-skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()

			dir := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, dir)
			}

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.stubs != nil {
				tt.stubs(reg)
			}

			opts := tt.opts(ios, dir, reg)
			err := updateRun(opts)

			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantStderr != "" {
				assert.Contains(t, stderr.String(), tt.wantStderr)
			}
			if tt.wantStdout != "" {
				assert.Contains(t, stdout.String(), tt.wantStdout)
			}
			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}

// If the staged contents cannot be installed after the existing entries
// have already been moved aside, the original skill directory must be
// restored byte-for-byte and its inode must be preserved.
func TestSwapDirectoryContents_RollsBackOnFailure(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "code-review")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("original"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "extra.txt"), []byte("keep me"), 0o644))
	subdir := filepath.Join(dest, "examples")
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "demo.txt"), []byte("demo"), 0o644))

	destBefore, err := os.Stat(dest)
	require.NoError(t, err)

	// Point src at a path that does not exist so the staged ReadDir fails
	// after the existing entries have already been moved aside. This is the
	// only deterministic, portable way to exercise the rollback branch from
	// outside the swap.
	src := filepath.Join(parent, "does-not-exist")

	err = swapDirectoryContents(dest, src)
	require.Error(t, err, "swap should fail when staged dir cannot be read")

	destAfter, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, os.SameFile(destBefore, destAfter), "dest directory identity must be preserved across rollback")

	content, readErr := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	require.NoError(t, readErr)
	assert.Equal(t, "original", string(content), "original SKILL.md must be restored")
	extra, readErr := os.ReadFile(filepath.Join(dest, "extra.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "keep me", string(extra), "original extra.txt must be restored")
	demo, readErr := os.ReadFile(filepath.Join(subdir, "demo.txt"))
	require.NoError(t, readErr)
	assert.Equal(t, "demo", string(demo), "original nested subdir must be restored intact")

	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	var leftovers []string
	for _, e := range entries {
		if e.Name() != "code-review" {
			leftovers = append(leftovers, e.Name())
		}
	}
	assert.Empty(t, leftovers, "no staging or backup directories should remain after rollback")
}

// The skill directory's own inode must survive an update so symlinks,
// bind mounts, and other external references pointing at it remain
// valid. Per-entry rename swaps satisfy this; replacing the directory
// itself would not.
func TestSwapDirectoryContents_PreservesDestInode(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "code-review")
	require.NoError(t, os.MkdirAll(dest, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "old.txt"), []byte("old"), 0o644))

	src := filepath.Join(parent, "staged")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "new.txt"), []byte("new"), 0o644))

	destBefore, err := os.Stat(dest)
	require.NoError(t, err)

	require.NoError(t, swapDirectoryContents(dest, src))

	destAfter, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, os.SameFile(destBefore, destAfter), "dest directory identity must be preserved")

	assert.NoFileExists(t, filepath.Join(dest, "old.txt"), "stale files must be removed")
	content, err := os.ReadFile(filepath.Join(dest, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content), "staged content must be installed")
}
