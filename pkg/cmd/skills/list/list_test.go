package list

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wantOpts ListOptions
		wantJSON bool
		wantErr  string
	}{
		{
			name:     "no flags",
			cli:      "",
			wantOpts: ListOptions{},
		},
		{
			name: "agent and scope filters",
			cli:  "--agent github-copilot --scope user",
			wantOpts: ListOptions{
				Agent:        "github-copilot",
				Scope:        "user",
				ScopeChanged: true,
			},
		},
		{
			name: "custom dir",
			cli:  "--dir ./skills",
			wantOpts: ListOptions{
				Dir: "./skills",
			},
		},
		{
			name:     "json fields",
			cli:      "--json skillName,sourceURL,scope,version,pinned,path",
			wantJSON: true,
		},
		{
			name:    "too many args",
			cli:     "extra",
			wantErr: "unknown command",
		},
		{
			name:    "invalid agent",
			cli:     "--agent unknown",
			wantErr: "invalid argument",
		},
		{
			name:    "invalid scope",
			cli:     "--scope org",
			wantErr: "invalid argument",
		},
		{
			name:    "dir and agent are mutually exclusive",
			cli:     "--dir ./skills --agent github-copilot",
			wantErr: "--dir and --agent cannot be used together",
		},
		{
			name:    "dir and scope are mutually exclusive",
			cli:     "--dir ./skills --scope user",
			wantErr: "--dir and --scope cannot be used together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				GitClient: &git.Client{},
			}

			var gotOpts *ListOptions
			cmd := NewCmdList(f, &telemetry.NoOpService{}, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			err = cmd.Execute()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, gotOpts)
			assert.Equal(t, tt.wantOpts.Agent, gotOpts.Agent)
			assert.Equal(t, tt.wantOpts.Scope, gotOpts.Scope)
			assert.Equal(t, tt.wantOpts.ScopeChanged, gotOpts.ScopeChanged)
			assert.Equal(t, tt.wantOpts.Dir, gotOpts.Dir)
			if tt.wantJSON {
				assert.NotNil(t, gotOpts.Exporter)
			}
		})
	}
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, repoDir, homeDir string)
		opts       func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions
		wantStdout string
		wantJSON   string
		wantErr    string
		verify     func(t *testing.T, stdout string, spy *telemetry.CommandRecorderSpy)
	}{
		{
			name: "lists project skill for selected shared agent",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, repoDir, ".agents/skills/git-commit", remoteSkillFrontmatter("git-commit", "skills/git-commit", "refs/tags/v1.0.0", ""))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Agent:     "cursor",
					Scope:     "project",
				}
			},
			wantStdout: "git-commit\tcursor\tproject\tmonalisa/skills-repo\n",
			verify: func(t *testing.T, stdout string, spy *telemetry.CommandRecorderSpy) {
				require.Len(t, spy.Events, 1)
				event := spy.Events[0]
				assert.Equal(t, "skill_list", event.Type)
				assert.Equal(t, "cursor", event.Dimensions["agent_hosts"])
				assert.Equal(t, "project", event.Dimensions["scope"])
				assert.Equal(t, int64(1), event.Measures["skill_count"])
			},
		},
		{
			name: "lists user skill as json",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, homeDir, ".copilot/skills/code-review", remoteSkillFrontmatter("code-review", "skills/code-review", "refs/tags/v2.0.0", "v2.0.0"))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields([]string{"skillName", "agentHosts", "scope", "sourceURL", "version", "pinned", "path"})
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Exporter:  exporter,
					Agent:     "github-copilot",
					Scope:     "user",
				}
			},
			wantJSON: fmt.Sprintf(`[
				{
					"skillName": "code-review",
					"agentHosts": ["github-copilot"],
					"scope": "user",
					"sourceURL": "https://github.com/monalisa/skills-repo",
					"version": "v2.0.0",
					"pinned": true,
					"path": %q
				}
			]`, filepath.Join("HOME", ".copilot", "skills", "code-review")),
			verify: func(t *testing.T, stdout string, spy *telemetry.CommandRecorderSpy) {
				assert.Equal(t, "json", spy.Events[0].Dimensions["format"])
			},
		},
		{
			name: "preserves tenant host in json source url",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, homeDir, ".copilot/skills/tenant-skill", remoteSkillFrontmatterForRepo("tenant-skill", "https://octocorp.ghe.com/monalisa/skills-repo", "skills/tenant-skill", "refs/heads/main", ""))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields([]string{"skillName", "sourceURL", "path"})
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Exporter:  exporter,
					Agent:     "github-copilot",
					Scope:     "user",
				}
			},
			wantJSON: fmt.Sprintf(`[
				{
					"skillName": "tenant-skill",
					"sourceURL": "https://octocorp.ghe.com/monalisa/skills-repo",
					"path": %q
				}
			]`, filepath.Join("HOME", ".copilot", "skills", "tenant-skill")),
		},
		{
			name: "custom directory with local metadata",
			setup: func(t *testing.T, repoDir, homeDir string) {
				customDir := filepath.Join(repoDir, "custom-skills")
				writeSkill(t, customDir, "local-helper", heredoc.Doc(`
					---
					name: local-helper
					metadata:
					  local-path: /src/local-helper
					---
					Body
				`))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Dir:       filepath.Join(repoDir, "custom-skills"),
				}
			},
			wantStdout: "local-helper\t-\tcustom\t/src/local-helper\n",
		},
		{
			name: "custom directory must exist",
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Dir:       filepath.Join(repoDir, "missing-skills"),
				}
			},
			wantErr: "could not access directory",
		},
		{
			name: "lists source skills in bare project skills directory as published",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, repoDir, "skills/gh", heredoc.Doc(`
					---
					name: gh
					description: GitHub CLI patterns
					---
					Body
				`))
				writeSkill(t, repoDir, "skills/gh-skill", heredoc.Doc(`
					---
					name: gh-skill
					description: GitHub Skill patterns
					---
					Body
				`))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Scope:     "project",
				}
			},
			wantStdout: "gh\tn/a (published)\tproject\t-\ngh-skill\tn/a (published)\tproject\t-\n",
		},
		{
			name: "lists openclaw project skill with install metadata",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, repoDir, "skills/openclaw-helper", remoteSkillFrontmatter("openclaw-helper", "skills/openclaw-helper", "refs/heads/main", ""))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Agent:     "openclaw",
					Scope:     "project",
				}
			},
			wantStdout: "openclaw-helper\topenclaw\tproject\tmonalisa/skills-repo\n",
		},
		{
			name: "recovers namespaced skill name from source path",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, repoDir, ".agents/skills/xlsx-pro", remoteSkillFrontmatter("xlsx-pro", "skills/bob/xlsx-pro", "refs/heads/main", ""))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Agent:     "github-copilot",
					Scope:     "project",
				}
			},
			wantStdout: "bob/xlsx-pro\tgithub-copilot\tproject\tmonalisa/skills-repo\n",
		},
		{
			name: "recovers plugin skill name from source path",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, repoDir, ".agents/skills/foo", remoteSkillFrontmatter("foo", "plugins/myplugin/skills/foo", "refs/heads/main", ""))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Agent:     "github-copilot",
					Scope:     "project",
				}
			},
			wantStdout: "myplugin/foo\tgithub-copilot\tproject\tmonalisa/skills-repo\n",
		},
		{
			name: "partial metadata has empty json source url",
			setup: func(t *testing.T, repoDir, homeDir string) {
				writeSkill(t, repoDir, ".agents/skills/partial", heredoc.Doc(`
					---
					name: partial
					metadata:
					  github-ref: refs/heads/main
					---
					Body
				`))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields([]string{"skillName", "sourceURL", "version", "pinned"})
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Exporter:  exporter,
					Agent:     "github-copilot",
					Scope:     "project",
				}
			},
			wantJSON: `[
				{
					"skillName": "partial",
					"sourceURL": "",
					"version": "main",
					"pinned": false
				}
			]`,
		},
		{
			name: "no installed skills returns no results",
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Agent:     "github-copilot",
					Scope:     "project",
				}
			},
			wantErr: "no installed skills found",
		},
		{
			name: "no installed skills with json returns empty array",
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields([]string{"skillName"})
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Exporter:  exporter,
					Agent:     "github-copilot",
					Scope:     "project",
				}
			},
			wantJSON: "[]",
		},
		{
			name: "lists skill whose SKILL.md is a symlink to a regular file",
			setup: func(t *testing.T, repoDir, homeDir string) {
				customDir := filepath.Join(repoDir, "custom-skills")
				skillDir := filepath.Join(customDir, "linked")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				target := filepath.Join(repoDir, "target.md")
				require.NoError(t, os.WriteFile(target, []byte("---\nname: linked\nmetadata:\n  local-path: /src/linked\n---\nBody\n"), 0o644))
				require.NoError(t, os.Symlink(target, filepath.Join(skillDir, "SKILL.md")))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Dir:       filepath.Join(repoDir, "custom-skills"),
				}
			},
			wantStdout: "linked\t-\tcustom\t/src/linked\n",
		},
		{
			name: "skips skill whose SKILL.md is not a regular file",
			setup: func(t *testing.T, repoDir, homeDir string) {
				customDir := filepath.Join(repoDir, "custom-skills")
				skillDir := filepath.Join(customDir, "bogus")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				targetDir := filepath.Join(repoDir, "target-dir")
				require.NoError(t, os.MkdirAll(targetDir, 0o755))
				require.NoError(t, os.Symlink(targetDir, filepath.Join(skillDir, "SKILL.md")))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Dir:       filepath.Join(repoDir, "custom-skills"),
				}
			},
			wantErr: "no installed skills found",
		},
		{
			name: "sanitizes terminal escapes from skill frontmatter",
			setup: func(t *testing.T, repoDir, homeDir string) {
				customDir := filepath.Join(repoDir, "custom-skills")
				writeSkill(t, customDir, "helper", heredoc.Doc(`
					---
					name: helper
					metadata:
					  local-path: "/src/\x1b[33munsanitized-src\x1b[0m"
					  github-path: "skills/\x1b[31munsanitized-name\x1b[0m/SKILL.md"
					---
					Body
				`))
			},
			opts: func(ios *iostreams.IOStreams, repoDir, homeDir string, spy *telemetry.CommandRecorderSpy) *ListOptions {
				return &ListOptions{
					IO:        ios,
					Telemetry: spy,
					GitClient: &git.Client{RepoDir: repoDir},
					Dir:       filepath.Join(repoDir, "custom-skills"),
				}
			},
			wantStdout: "^[[31munsanitized-name^[[0m\t-\tcustom\t/src/^[[33munsanitized-src^[[0m\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir := t.TempDir()
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			t.Setenv("USERPROFILE", homeDir)

			if tt.setup != nil {
				tt.setup(t, repoDir, homeDir)
			}

			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(false)
			spy := &telemetry.CommandRecorderSpy{}
			opts := tt.opts(ios, repoDir, homeDir, spy)

			err := listRun(opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.wantJSON != "" {
				expected := tt.wantJSON
				expected = strings.ReplaceAll(expected, "HOME", strings.ReplaceAll(homeDir, `\`, `\\`))
				assert.JSONEq(t, expected, stdout.String())
			} else {
				assert.Equal(t, tt.wantStdout, stdout.String())
			}
			if tt.verify != nil {
				tt.verify(t, stdout.String(), spy)
			}
		})
	}
}

func TestRenderTableUsesAgentHeader(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	err := renderTable(ios, []listedSkill{{
		skillName:    "git-commit",
		agentHostIDs: []string{"github-copilot", "cursor"},
		scope:        "project",
		source:       "monalisa/skills-repo",
		version:      "v1.0.0",
	}})

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "AGENT")
	assert.Contains(t, stdout.String(), "github-copilot, cursor")
	assert.NotContains(t, stdout.String(), "HOST")
}

func writeSkill(t *testing.T, baseDir, relDir, content string) {
	t.Helper()
	skillDir := filepath.Join(baseDir, filepath.FromSlash(relDir))
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))
}

func remoteSkillFrontmatter(name, sourcePath, ref, pinned string) string {
	return remoteSkillFrontmatterForRepo(name, "https://github.com/monalisa/skills-repo", sourcePath, ref, pinned)
}

func remoteSkillFrontmatterForRepo(name, repoURL, sourcePath, ref, pinned string) string {
	pinnedLine := ""
	if pinned != "" {
		pinnedLine = fmt.Sprintf("  github-pinned: %s\n", pinned)
	}
	return fmt.Sprintf(heredoc.Doc(`
		---
		name: %s
		metadata:
		  github-repo: %s
		  github-ref: %s
		  github-tree-sha: abc123
		  github-path: %s
		%s---
		Body
	`), name, repoURL, ref, sourcePath, pinnedLine)
}
