package publish

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestGitClient returns a git.Client with a fake git path to avoid real git resolution.
func newTestGitClient() *git.Client {
	return &git.Client{GitPath: "some/path/git"}
}

// stubGitRemote registers CommandStubber stubs for git remote detection.
func stubGitRemote(cs *run.CommandStubber, remoteURLs map[string]string) {
	var remoteLines string
	for name, url := range remoteURLs {
		remoteLines += fmt.Sprintf("%[1]s\t%[2]s (fetch)\n%[1]s\t%[2]s (push)\n", name, url)
	}
	cs.Register(`git( .+)? remote -v`, 0, remoteLines)
	cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
	for name, url := range remoteURLs {
		cs.Register(fmt.Sprintf(`git( .+)? remote get-url -- %s`, regexp.QuoteMeta(name)), 0, url+"\n")
	}
}

// stubEnsurePushed registers stubs for ensurePushed + runPublishRelease CurrentBranch calls.
func stubEnsurePushed(cs *run.CommandStubber, branch string) {
	cs.Register(`git( .+)? symbolic-ref --quiet HEAD`, 0, "refs/heads/"+branch+"\n")
	cs.Register(`git( .+)? rev-list --count @\{push\}\.\.HEAD`, 0, "0\n")
	cs.Register(`git( .+)? symbolic-ref --quiet HEAD`, 0, "refs/heads/"+branch+"\n")
}

// stubAllSecureRemote registers the standard stubs for a fully-configured remote
// repo (topics, tags, rulesets, security) so publishRun skips all remote warnings.
func stubAllSecureRemote(reg *httpmock.Registry, owner, repo string) {
	reg.Register(
		httpmock.REST("GET", "repos/"+owner+"/"+repo+"/topics"),
		httpmock.JSONResponse(map[string]interface{}{
			"names": []string{"agent-skills"},
		}),
	)
	reg.Register(
		httpmock.REST("GET", "repos/"+owner+"/"+repo+"/tags"),
		httpmock.JSONResponse([]map[string]interface{}{
			{"name": "v1.0.0"},
		}),
	)
	reg.Register(
		httpmock.REST("GET", "repos/"+owner+"/"+repo+"/rulesets"),
		httpmock.JSONResponse([]map[string]interface{}{
			{"id": 1, "name": "tags", "target": "tag", "enforcement": "active"},
		}),
	)
	reg.Register(
		httpmock.REST("GET", "repos/"+owner+"/"+repo),
		httpmock.JSONResponse(map[string]interface{}{
			"security_and_analysis": map[string]interface{}{
				"secret_scanning":                 map[string]interface{}{"status": "enabled"},
				"secret_scanning_push_protection": map[string]interface{}{"status": "enabled"},
			},
		}),
	)
}

func TestNewCmdPublish(t *testing.T) {
	tests := []struct {
		name      string
		cli       string
		wantsErr  bool
		wantsOpts PublishOptions
	}{
		{
			name:     "fix and dry-run are mutually exclusive",
			cli:      "./monalisa-skills --dry-run --fix --tag v1.0.0",
			wantsErr: true,
		},
		{
			name: "fix flag only",
			cli:  "--fix",
			wantsOpts: PublishOptions{
				Fix: true,
			},
		},
		{
			name: "directory only",
			cli:  "./octocat-repo",
			wantsOpts: PublishOptions{
				Dir: "./octocat-repo",
			},
		},
		{
			name:      "no args leaves dir empty",
			cli:       "",
			wantsOpts: PublishOptions{},
		},
		{
			name: "dry-run flag only",
			cli:  "--dry-run",
			wantsOpts: PublishOptions{
				DryRun: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := cmdutil.Factory{IOStreams: ios}

			var gotOpts *PublishOptions
			cmd := NewCmdPublish(&f, func(opts *PublishOptions) error {
				gotOpts = opts
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err = cmd.Execute()
			if tt.wantsErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, gotOpts)
			assert.Equal(t, tt.wantsOpts.Dir, gotOpts.Dir)
			assert.Equal(t, tt.wantsOpts.DryRun, gotOpts.DryRun)
			assert.Equal(t, tt.wantsOpts.Fix, gotOpts.Fix)
			assert.Equal(t, tt.wantsOpts.Tag, gotOpts.Tag)
		})
	}
}

func TestPublishRun_UnsupportedHost(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "test-skill", heredoc.Doc(`
		---
		name: test-skill
		description: A test skill
		---
		Body.
	`))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	stubGitRemote(cs, map[string]string{"origin": "https://github.com/monalisa/skills-repo.git"})

	ios, _, _, _ := iostreams.Test()
	err := publishRun(&PublishOptions{
		IO:         ios,
		Dir:        dir,
		GitClient:  newTestGitClient(),
		HttpClient: func() (*http.Client, error) { return nil, nil },
		host:       "acme.ghes.com",
	})
	require.ErrorContains(t, err, "supports only github.com")
}

func TestPublishRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		setup      func(t *testing.T, dir string)
		stubs      func(*httpmock.Registry)
		cmdStubs   func(*run.CommandStubber)
		opts       func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions
		verify     func(t *testing.T, dir string)
		wantErr    string
		wantStdout string
		wantStderr string
	}{
		{
			name:  "no skills found",
			setup: func(_ *testing.T, _ string) {},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir}
			},
			wantErr: "no skills found",
		},
		{
			name: "empty skills directory has no discoverable skills",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills", "empty-skill"), 0o755))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir}
			},
			wantErr: "no skills found",
		},
		{
			name: "missing name in frontmatter",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "git-commit", heredoc.Doc(`
					---
					description: A skill for writing good git commits
					---
					Body text.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir}
			},
			wantErr:    "validation failed",
			wantStdout: "missing required field: name",
		},
		{
			name: "name does not match directory",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "git-commit", heredoc.Doc(`
					---
					name: wrong-name
					description: A skill
					---
					Body.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir}
			},
			wantErr:    "validation failed",
			wantStdout: "does not match directory name",
		},
		{
			name: "non-spec-compliant name",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "My_Skill", heredoc.Doc(`
					---
					name: My_Skill
					description: A skill with non-compliant name
					---
					Body.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir}
			},
			wantErr:    "validation failed",
			wantStdout: "naming convention",
		},
		{
			name:  "root-level skill discovered and validated",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				// Create a root-level skill (*/SKILL.md convention)
				skillDir := filepath.Join(dir, "my-root-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: my-root-skill
					description: A root-level skill
					license: MIT
					---
					Body.
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					DryRun:     true,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "1 skill(s) validated successfully",
			wantStderr: "Dry run complete",
		},
		{
			name:  "namespaced skill discovered and validated",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				// Create a namespaced skill (skills/{scope}/*/SKILL.md convention)
				skillDir := filepath.Join(dir, "skills", "monalisa", "scoped-skill")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(heredoc.Doc(`
					---
					name: scoped-skill
					description: A namespaced skill
					license: MIT
					---
					Body.
				`)), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					DryRun:     true,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "1 skill(s) validated successfully",
			wantStderr: "Dry run complete",
		},
		{
			name:  "valid skill dry-run passes validation",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "good-skill", heredoc.Doc(`
					---
					name: good-skill
					description: A good skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					DryRun:     true,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "1 skill(s) validated successfully",
			wantStderr: "Dry run complete",
		},
		{
			name:  "valid skill with --tag publishes release",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "git-commit", heredoc.Doc(`
					---
					name: git-commit
					description: A skill for writing good git commits
					allowed-tools: git
					license: MIT
					---
					You are a git commit expert.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
				// topic already present, so no PUT needed
				// immutable releases check
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.JSONResponse(map[string]interface{}{"enabled": true}),
				)
				// default branch for branch comparison
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}),
				)
				// create release
				reg.Register(
					httpmock.REST("POST", "repos/monalisa/skills-repo/releases"),
					httpmock.JSONResponse(map[string]interface{}{
						"html_url": "https://github.com/monalisa/skills-repo/releases/tag/v1.0.1",
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				stubEnsurePushed(cs, "main")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:  ios,
					Dir: dir,
					Tag: "v1.0.1",
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, def bool) (bool, error) { return true, nil },
					},
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "Published v1.0.1",
		},
		{
			name: "strip metadata with --fix",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "test-skill", heredoc.Doc(`
					---
					name: test-skill
					description: A test skill
					metadata:
					    github-owner: someone
					    github-repo: something
					    github-ref: v1.0.0
					    github-sha: abc123
					    github-tree-sha: def456
					---
					Body.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir, Fix: true}
			},
			wantStdout: "stripped install metadata",
			wantStderr: "Fixed 1 file(s). Review and commit the changes",
			verify: func(t *testing.T, dir string) {
				t.Helper()
				fixed, err := os.ReadFile(filepath.Join(dir, "skills", "test-skill", "SKILL.md"))
				require.NoError(t, err)
				fixedStr := string(fixed)
				assert.NotContains(t, fixedStr, "github-owner")
				assert.NotContains(t, fixedStr, "github-sha")
				assert.NotContains(t, fixedStr, "metadata:")
			},
		},
		{
			name: "metadata without --fix errors with hint",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "test-skill", heredoc.Doc(`
					---
					name: test-skill
					description: A test skill
					metadata:
					    github-owner: someone
					    github-sha: abc123
					---
					Body.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir, Fix: false}
			},
			wantErr:    "validation failed",
			wantStdout: "--fix",
		},
		{
			name: "missing license warning",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "no-license", heredoc.Doc(`
					---
					name: no-license
					description: A skill without license
					---
					Body.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir}
			},
			wantStdout: "license",
		},
		{
			name: "allowed-tools array error",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "bad-tools", heredoc.Doc(`
					---
					name: bad-tools
					description: A skill with array allowed-tools
					allowed-tools:
					  - git
					  - curl
					---
					Body.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{IO: ios, Dir: dir}
			},
			wantErr:    "validation failed",
			wantStdout: "allowed-tools must be a string",
		},
		{
			name: "security warnings when features disabled",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/octocat/secure-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{
						"names": []string{"agent-skills"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/secure-repo/tags"),
					httpmock.JSONResponse([]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/secure-repo/rulesets"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"id": 1, "name": "branch-only", "target": "branch", "enforcement": "active"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/secure-repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"security_and_analysis": map[string]interface{}{
							"secret_scanning":                 map[string]interface{}{"status": "disabled"},
							"secret_scanning_push_protection": map[string]interface{}{"status": "disabled"},
						},
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/octocat/secure-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "secret scanning is not enabled",
		},
		{
			name: "tag protection warning",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/octocat/tag-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{
						"names": []string{"agent-skills"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/tag-repo/tags"),
					httpmock.JSONResponse([]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/tag-repo/rulesets"),
					httpmock.JSONResponse([]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/tag-repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"security_and_analysis": map[string]interface{}{
							"secret_scanning":                 map[string]interface{}{"status": "enabled"},
							"secret_scanning_push_protection": map[string]interface{}{"status": "enabled"},
						},
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/octocat/tag-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "tag protection",
		},
		{
			name:  "code files trigger code scanning info",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "code-skill", heredoc.Doc(`
					---
					name: code-skill
					description: A skill with code
					license: MIT
					---
					Body.
				`))
				scriptDir := filepath.Join(dir, "skills", "code-skill", "scripts")
				require.NoError(t, os.MkdirAll(scriptDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(scriptDir, "helper.sh"), []byte("#!/bin/bash"), 0o644))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/octocat/code-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{
						"names": []string{"agent-skills"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/code-repo/tags"),
					httpmock.JSONResponse([]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/code-repo/rulesets"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"id": 1, "name": "tags", "target": "tag", "enforcement": "active"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/code-repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"security_and_analysis": map[string]interface{}{
							"secret_scanning":                 map[string]interface{}{"status": "enabled"},
							"secret_scanning_push_protection": map[string]interface{}{"status": "enabled"},
						},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/code-repo/code-scanning/alerts"),
					httpmock.StatusStringResponse(404, "not found"),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/octocat/code-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					DryRun:     true,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStderr: "code scanning",
		},
		{
			name:  "manifest files trigger dependabot info",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "dep-skill", heredoc.Doc(`
					---
					name: dep-skill
					description: A skill with manifests
					license: MIT
					---
					Body.
				`))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "skills", "dep-skill", "package.json"),
					[]byte("{}"), 0o644,
				))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/octocat/dep-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{
						"names": []string{"agent-skills"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/dep-repo/tags"),
					httpmock.JSONResponse([]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/dep-repo/rulesets"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"id": 1, "name": "tags", "target": "tag", "enforcement": "active"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/dep-repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"security_and_analysis": map[string]interface{}{
							"secret_scanning":                 map[string]interface{}{"status": "enabled"},
							"secret_scanning_push_protection": map[string]interface{}{"status": "enabled"},
						},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/octocat/dep-repo/vulnerability-alerts"),
					httpmock.StatusStringResponse(404, "not found"),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/octocat/dep-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					DryRun:     true,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStderr: "Dependabot",
		},
		{
			name:  "installed skill dirs not gitignored warns",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", "installed"), 0o755))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? check-ignore -q -- .agents/skills`, 1, "")
				cs.Register(`git( .+)? remote -v`, 0, "")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
				cs.Register(`git( .+)? rev-parse --git-dir`, 0, ".git\n")
				cs.Register(`git( .+)? remote -v`, 0, "")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:        ios,
					Dir:       dir,
					GitClient: &git.Client{GitPath: "some/path/git", RepoDir: dir},
				}
			},
			wantStdout: "may contain installed skills that are not gitignored",
		},
		{
			name: "installed skill dirs gitignored no warning",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", "installed"), 0o755))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? check-ignore -q -- .agents/skills`, 0, "")
				cs.Register(`git( .+)? remote -v`, 0, "")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
				cs.Register(`git( .+)? rev-parse --git-dir`, 0, ".git\n")
				cs.Register(`git( .+)? remote -v`, 0, "")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:        ios,
					Dir:       dir,
					GitClient: &git.Client{GitPath: "some/path/git", RepoDir: dir},
				}
			},
			wantStdout: "no git remote",
			verify: func(t *testing.T, dir string) {
				t.Helper()
				// The key assertion: .gitignored dirs should NOT produce a warning
			},
		},
		{
			name: "installed skill dirs git error warns about unverified status",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "skills", "installed"), 0o755))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? check-ignore -q -- .agents/skills`, 128, "")
				cs.Register(`git( .+)? remote -v`, 0, "")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
				cs.Register(`git( .+)? rev-parse --git-dir`, 0, ".git\n")
				cs.Register(`git( .+)? remote -v`, 0, "")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:        ios,
					Dir:       dir,
					GitClient: &git.Client{GitPath: "some/path/git", RepoDir: dir},
				}
			},
			wantStdout: "may contain installed skills that are not gitignored",
		},
		{
			name: "no GitHub remote warns",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://gitlab.com/hubot/bar.git",
				})
				cs.Register(`git( .+)? rev-parse --git-dir`, 0, ".git\n")
				cs.Register(`git( .+)? remote -v`, 0, "origin\thttps://gitlab.com/hubot/bar.git (fetch)\norigin\thttps://gitlab.com/hubot/bar.git (push)\n")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
				cs.Register(fmt.Sprintf(`git( .+)? remote get-url -- %s`, regexp.QuoteMeta("origin")), 0, "https://gitlab.com/hubot/bar.git\n")
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:        ios,
					Dir:       dir,
					GitClient: &git.Client{GitPath: "some/path/git", RepoDir: dir},
				}
			},
			wantStdout: "not a GitHub repository",
		},
		{
			name:  "fallback remote detection uses non-origin GitHub remote",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "octocat", "repo")
			},
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? remote -v`, 0, "origin\thttps://gitlab.com/hubot/bar.git (fetch)\norigin\thttps://gitlab.com/hubot/bar.git (push)\nupstream\tgit@github.com:octocat/repo.git (fetch)\nupstream\tgit@github.com:octocat/repo.git (push)\n")
				cs.Register(`git( .+)? config --get-regexp \^remote\\\.`, 1, "")
				// upstream sorts first (score 3 > 1), so only upstream's get-url is called
				cs.Register(fmt.Sprintf(`git( .+)? remote get-url -- %s`, regexp.QuoteMeta("upstream")), 0, "git@github.com:octocat/repo.git\n")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					DryRun:     true,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStderr: "octocat/repo",
		},
		{
			name:  "publish adds missing topic via --tag",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				// topic missing
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{
						"names": []string{"golang"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/tags"),
					httpmock.JSONResponse([]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/rulesets"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"id": 1, "name": "tags", "target": "tag", "enforcement": "active"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"security_and_analysis": map[string]interface{}{
							"secret_scanning":                 map[string]interface{}{"status": "enabled"},
							"secret_scanning_push_protection": map[string]interface{}{"status": "enabled"},
						},
					}),
				)
				// addAgentSkillsTopic fetches topics again then PUTs
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{
						"names": []string{"golang"},
					}),
				)
				reg.Register(
					httpmock.REST("PUT", "repos/monalisa/skills-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{}),
				)
				// immutable releases
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.JSONResponse(map[string]interface{}{"enabled": true}),
				)
				// default branch
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}),
				)
				// create release
				reg.Register(
					httpmock.REST("POST", "repos/monalisa/skills-repo/releases"),
					httpmock.JSONResponse(map[string]interface{}{
						"html_url": "https://github.com/monalisa/skills-repo/releases/tag/v1.0.0",
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				stubEnsurePushed(cs, "main")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:  ios,
					Dir: dir,
					Tag: "v1.0.0",
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, def bool) (bool, error) { return true, nil },
					},
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
		},
		{
			name: "tag suggestion uses existing tags",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{
						"names": []string{"agent-skills"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/tags"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"name": "v2.3.4"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/rulesets"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"id": 1, "name": "tags", "target": "tag", "enforcement": "active"},
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"security_and_analysis": map[string]interface{}{
							"secret_scanning":                 map[string]interface{}{"status": "enabled"},
							"secret_scanning_push_protection": map[string]interface{}{"status": "enabled"},
						},
					}),
				)
				// immutable releases
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.JSONResponse(map[string]interface{}{"enabled": true}),
				)
				// default branch
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}),
				)
				// create release with the suggested v2.3.5 tag
				reg.Register(
					httpmock.REST("POST", "repos/monalisa/skills-repo/releases"),
					httpmock.JSONResponse(map[string]interface{}{
						"html_url": "https://github.com/monalisa/skills-repo/releases/tag/v2.3.5",
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				stubEnsurePushed(cs, "main")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					Tag:        "v2.3.5",
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "Published v2.3.5",
		},
		{
			name: "duplicate tag errors",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				cs.Register(`git( .+)? symbolic-ref --quiet HEAD`, 0, "refs/heads/main\n")
				cs.Register(`git( .+)? rev-list --count @\{push\}\.\.HEAD`, 0, "0\n")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					Tag:        "v1.0.0", // same as stubAllSecureRemote's existing tag
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantErr: "tag v1.0.0 already exists",
		},
		{
			name: "valid skill non-tty plain output",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "git-commit", heredoc.Doc(`
					---
					name: git-commit
					description: A skill for writing good git commits
					allowed-tools: git
					license: MIT
					---
					You are a git commit expert.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:         ios,
					Dir:        dir,
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "ok",
		},
		{
			name: "no remote and non-tty shows validation passed message",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			opts: func(ios *iostreams.IOStreams, dir string, _ *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:  ios,
					Dir: dir,
				}
			},
			wantStdout: "ok",
		},
		{
			name:  "interactive publish with topic and semver tag",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				// No topic yet, first GET for diagnostic check
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{"names": []string{}}),
				)
				// Second GET inside addAgentSkillsTopic
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/topics"),
					httpmock.JSONResponse(map[string]interface{}{"names": []string{}}),
				)
				// Add topic
				reg.Register(
					httpmock.REST("PUT", "repos/monalisa/skills-repo/topics"),
					httpmock.StringResponse("{}"),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/tags"),
					httpmock.JSONResponse([]map[string]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/rulesets"),
					httpmock.JSONResponse([]map[string]interface{}{}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"default_branch": "main",
						"security_and_analysis": map[string]interface{}{
							"secret_scanning":                 map[string]interface{}{"status": "enabled"},
							"secret_scanning_push_protection": map[string]interface{}{"status": "enabled"},
						},
					}),
				)
				// Immutable releases already enabled
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.JSONResponse(map[string]interface{}{"enabled": true}),
				)
				// Create release
				reg.Register(
					httpmock.REST("POST", "repos/monalisa/skills-repo/releases"),
					httpmock.JSONResponse(map[string]interface{}{
						"html_url": "https://github.com/monalisa/skills-repo/releases/tag/v1.0.0",
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				stubEnsurePushed(cs, "main")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				confirmCall := 0
				return &PublishOptions{
					IO:  ios,
					Dir: dir,
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, def bool) (bool, error) {
							confirmCall++
							return true, nil // accept topic + final confirm
						},
						SelectFunc: func(msg string, def string, opts []string) (int, error) {
							return 0, nil // semver strategy
						},
						InputFunc: func(msg string, def string) (string, error) {
							return "v1.0.0", nil // accept suggested tag
						},
					},
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "Published v1.0.0",
		},
		{
			name:  "interactive publish with custom tag",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.JSONResponse(map[string]interface{}{"enabled": true}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}),
				)
				reg.Register(
					httpmock.REST("POST", "repos/monalisa/skills-repo/releases"),
					httpmock.JSONResponse(map[string]interface{}{
						"html_url": "https://github.com/monalisa/skills-repo/releases/tag/beta-1",
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				stubEnsurePushed(cs, "main")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:  ios,
					Dir: dir,
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, def bool) (bool, error) {
							return true, nil
						},
						SelectFunc: func(msg string, def string, opts []string) (int, error) {
							return 1, nil // custom tag strategy
						},
						InputFunc: func(msg string, def string) (string, error) {
							return "beta-1", nil
						},
					},
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "Published beta-1",
		},
		{
			name:  "interactive publish declined at final confirm",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.JSONResponse(map[string]interface{}{"enabled": true}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				stubEnsurePushed(cs, "main")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				confirmCall := 0
				return &PublishOptions{
					IO:  ios,
					Dir: dir,
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, def bool) (bool, error) {
							confirmCall++
							if confirmCall >= 1 {
								return false, nil // decline final confirm
							}
							return true, nil
						},
						SelectFunc: func(msg string, def string, opts []string) (int, error) {
							return 0, nil
						},
						InputFunc: func(msg string, def string) (string, error) {
							return "v1.0.1", nil
						},
					},
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantErr:    "CancelError",
			wantStderr: "Publish cancelled",
		},
		{
			name:  "interactive immutable releases prompt",
			isTTY: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				writeSkill(t, dir, "my-skill", heredoc.Doc(`
					---
					name: my-skill
					description: A skill
					license: MIT
					---
					Body.
				`))
			},
			stubs: func(reg *httpmock.Registry) {
				stubAllSecureRemote(reg, "monalisa", "skills-repo")
				// Immutable releases NOT enabled
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.JSONResponse(map[string]interface{}{"enabled": false}),
				)
				// Enable immutable releases
				reg.Register(
					httpmock.REST("PATCH", "repos/monalisa/skills-repo/immutable-releases"),
					httpmock.StringResponse("{}"),
				)
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/skills-repo"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}),
				)
				reg.Register(
					httpmock.REST("POST", "repos/monalisa/skills-repo/releases"),
					httpmock.JSONResponse(map[string]interface{}{
						"html_url": "https://github.com/monalisa/skills-repo/releases/tag/v1.0.1",
					}),
				)
			},
			cmdStubs: func(cs *run.CommandStubber) {
				stubGitRemote(cs, map[string]string{
					"origin": "https://github.com/monalisa/skills-repo.git",
				})
				stubEnsurePushed(cs, "main")
			},
			opts: func(ios *iostreams.IOStreams, dir string, reg *httpmock.Registry) *PublishOptions {
				t.Helper()
				return &PublishOptions{
					IO:  ios,
					Dir: dir,
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(msg string, def bool) (bool, error) {
							return true, nil // accept all confirms (immutable + final)
						},
						SelectFunc: func(msg string, def string, opts []string) (int, error) {
							return 0, nil
						},
						InputFunc: func(msg string, def string) (string, error) {
							return "v1.0.1", nil
						},
					},
					GitClient:  newTestGitClient(),
					HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
					host:       "github.com",
				}
			},
			wantStdout: "Enabled immutable releases",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.stubs != nil {
				tt.stubs(reg)
			}

			if tt.cmdStubs != nil {
				cs, cmdTeardown := run.Stub()
				defer cmdTeardown(t)
				tt.cmdStubs(cs)
			}

			opts := tt.opts(ios, dir, reg)
			err := publishRun(opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantStdout != "" {
				assert.Contains(t, stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != "" {
				assert.Contains(t, stderr.String(), tt.wantStderr)
			}
			if tt.verify != nil {
				tt.verify(t, dir)
			}
		})
	}
}

func TestDetectGitHubRemote_UsesDir(t *testing.T) {
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	stubGitRemote(cs, map[string]string{
		"origin": "https://github.com/monalisa/target-repo.git",
	})

	cwdRepo := t.TempDir()
	targetRepo := t.TempDir()

	gitClient := &git.Client{GitPath: "some/path/git", RepoDir: cwdRepo}

	repo, err := detectGitHubRemote(gitClient, targetRepo)
	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, "monalisa", repo.Repo.RepoOwner())
	assert.Equal(t, "target-repo", repo.Repo.RepoName())
}

func TestPublishRun_DirArgUsesTargetRemote(t *testing.T) {
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	stubGitRemote(cs, map[string]string{
		"origin": "https://github.com/monalisa/target-repo.git",
	})

	cwdRepo := t.TempDir()
	targetRepo := t.TempDir()

	writeSkill(t, targetRepo, "my-skill", heredoc.Doc(`
		---
		name: my-skill
		description: A test skill
		license: MIT
		---
		Body text.
	`))

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)
	ios.SetStderrTTY(true)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	stubAllSecureRemote(reg, "monalisa", "target-repo")

	err := publishRun(&PublishOptions{
		IO:         ios,
		Dir:        targetRepo,
		DryRun:     true,
		GitClient:  &git.Client{GitPath: "some/path/git", RepoDir: cwdRepo},
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		host:       "github.com",
	})

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "1 skill(s) validated successfully")
}

// writeSkill creates skills/<name>/SKILL.md with the given content.
func writeSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	skillDir := filepath.Join(dir, "skills", name)
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))
}

func TestEnsurePushed(t *testing.T) {
	tests := []struct {
		name       string
		cmdStubs   func(*run.CommandStubber)
		wantErr    string
		wantStderr string
	}{
		{
			name: "no unpushed commits is a no-op",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? symbolic-ref --quiet HEAD`, 0, "refs/heads/main\n")
				cs.Register(`git( .+)? rev-list --count @\{push\}\.\.HEAD`, 0, "0\n")
			},
		},
		{
			name: "unpushed commits are pushed automatically",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? symbolic-ref --quiet HEAD`, 0, "refs/heads/main\n")
				cs.Register(`git( .+)? rev-list --count @\{push\}\.\.HEAD`, 0, "1\n")
				cs.Register(`git( .+)? push --set-upstream origin HEAD:refs/heads/main`, 0, "")
			},
			wantStderr: "Pushing main to origin",
		},
		{
			name: "new branch that has not been pushed is pushed automatically",
			cmdStubs: func(cs *run.CommandStubber) {
				cs.Register(`git( .+)? symbolic-ref --quiet HEAD`, 0, "refs/heads/feature\n")
				// rev-list fails when branch is not pushed
				cs.Register(`git( .+)? rev-list --count @\{push\}\.\.HEAD`, 1, "")
				cs.Register(`git( .+)? push --set-upstream origin HEAD:refs/heads/feature`, 0, "")
			},
			wantStderr: "Pushing feature to origin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			tt.cmdStubs(cs)

			workDir := t.TempDir()

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStderrTTY(true)

			opts := &PublishOptions{
				IO:        ios,
				GitClient: &git.Client{GitPath: "some/path/git", RepoDir: workDir},
			}

			err := ensurePushed(opts, workDir, "origin")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantStderr != "" {
				assert.Contains(t, stderr.String(), tt.wantStderr)
			}
		})
	}
}
