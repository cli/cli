package preview

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdPreview(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantRepo      string
		wantSkillName string
		wantVersion   string
		wantErr       bool
	}{
		{
			name:          "repo and skill",
			input:         "github/awesome-copilot my-skill",
			wantRepo:      "github/awesome-copilot",
			wantSkillName: "my-skill",
		},
		{
			name:          "repo and skill with version",
			input:         "github/awesome-copilot my-skill@v1.2.0",
			wantRepo:      "github/awesome-copilot",
			wantSkillName: "my-skill",
			wantVersion:   "v1.2.0",
		},
		{
			name:          "repo and skill with SHA",
			input:         "github/awesome-copilot my-skill@abc123def456",
			wantRepo:      "github/awesome-copilot",
			wantSkillName: "my-skill",
			wantVersion:   "abc123def456",
		},
		{
			name:     "repo only",
			input:    "github/awesome-copilot",
			wantRepo: "github/awesome-copilot",
		},
		{
			name:    "no args",
			input:   "",
			wantErr: true,
		},
		{
			name:    "too many args",
			input:   "a b c",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Prompter:  &prompter.PrompterMock{},
			}

			var gotOpts *PreviewOptions
			cmd := NewCmdPreview(f, &telemetry.NoOpService{}, func(opts *PreviewOptions) error {
				gotOpts = opts
				return nil
			})

			args, _ := shlex.Split(tt.input)
			cmd.SetArgs(args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			err := cmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, gotOpts.RepoArg)
			assert.Equal(t, tt.wantSkillName, gotOpts.SkillName)
			assert.Equal(t, tt.wantVersion, gotOpts.Version)
		})
	}
}

func TestPreviewRun(t *testing.T) {
	skillContent := heredoc.Doc(`
		---
		name: my-skill
		description: A test skill
		---
		# My Skill

		This is the skill content.
	`)
	encodedContent := base64.StdEncoding.EncodeToString([]byte(skillContent))

	tests := []struct {
		name       string
		opts       *PreviewOptions
		tty        bool
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantErr    string
	}{
		{
			name: "preview specific skill",
			tty:  true,
			opts: &PreviewOptions{
				repo:      ghrepo.New("github", "awesome-copilot"),
				SkillName: "my-skill",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/trees/abc123"),
					httpmock.StringResponse(`{
						"sha": "abc123",
						"truncated": false,
						"tree": [
							{"path": "skills", "type": "tree", "sha": "tree1"},
							{"path": "skills/my-skill", "type": "tree", "sha": "treeSHA"},
							{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blob123"}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/trees/treeSHA"),
					httpmock.StringResponse(`{
						"tree": [
							{"path": "SKILL.md", "type": "blob", "sha": "blob123", "size": 50}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/blobs/blob123"),
					httpmock.StringResponse(`{"sha": "blob123", "content": "`+encodedContent+`", "encoding": "base64"}`),
				)
			},
			wantStdout: "My Skill",
		},
		{
			name: "preview with display name match",
			tty:  true,
			opts: &PreviewOptions{
				repo:      ghrepo.New("owner", "repo"),
				SkillName: "ns/my-skill",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
					httpmock.StringResponse(`{
						"sha": "abc123",
						"truncated": false,
						"tree": [
							{"path": "skills", "type": "tree", "sha": "tree1"},
							{"path": "skills/ns", "type": "tree", "sha": "tree-ns"},
							{"path": "skills/ns/my-skill", "type": "tree", "sha": "treeSHA2"},
							{"path": "skills/ns/my-skill/SKILL.md", "type": "blob", "sha": "blob456"}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/trees/treeSHA2"),
					httpmock.StringResponse(`{
						"tree": [
							{"path": "SKILL.md", "type": "blob", "sha": "blob456", "size": 50}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/blobs/blob456"),
					httpmock.StringResponse(`{"sha": "blob456", "content": "`+encodedContent+`", "encoding": "base64"}`),
				)
			},
			wantStdout: "My Skill",
		},
		{
			name: "preview plugins skill matched by install name",
			tty:  true,
			opts: &PreviewOptions{
				repo:      ghrepo.New("owner", "repo"),
				SkillName: "aws-common/aws-mcp-setup",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
					httpmock.StringResponse(`{
						"sha": "abc123",
						"truncated": false,
						"tree": [
							{"path": "plugins", "type": "tree", "sha": "tree-plugins"},
							{"path": "plugins/aws-common", "type": "tree", "sha": "tree-awscommon"},
							{"path": "plugins/aws-common/skills", "type": "tree", "sha": "tree-awsskills"},
							{"path": "plugins/aws-common/skills/aws-mcp-setup", "type": "tree", "sha": "treeSHA3"},
							{"path": "plugins/aws-common/skills/aws-mcp-setup/SKILL.md", "type": "blob", "sha": "blob789"}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/trees/treeSHA3"),
					httpmock.StringResponse(`{
						"tree": [
							{"path": "SKILL.md", "type": "blob", "sha": "blob789", "size": 50}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/blobs/blob789"),
					httpmock.StringResponse(`{"sha": "blob789", "content": "`+encodedContent+`", "encoding": "base64"}`),
				)
			},
			wantStdout: "My Skill",
		},
		{
			name: "skill not found",
			tty:  true,
			opts: &PreviewOptions{
				repo:      ghrepo.New("owner", "repo"),
				SkillName: "nonexistent",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
					httpmock.StringResponse(`{
						"sha": "abc123",
						"truncated": false,
						"tree": [
							{"path": "skills/my-skill", "type": "tree", "sha": "tree2"},
							{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blob123"}
						]
					}`),
				)
			},
			wantErr: `skill "nonexistent" not found in owner/repo`,
		},
		{
			name: "no skill name non-interactive errors",
			tty:  false,
			opts: &PreviewOptions{
				repo: ghrepo.New("owner", "repo"),
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/releases/latest"),
					httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
					httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
					httpmock.StringResponse(`{
						"sha": "abc123",
						"truncated": false,
						"tree": [
							{"path": "skills/my-skill", "type": "tree", "sha": "tree2"},
							{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blob123"}
						]
					}`),
				)
			},
			wantErr: "must specify a skill name when not running interactively",
		},
		{
			name: "preview with explicit version",
			tty:  true,
			opts: &PreviewOptions{
				repo:      ghrepo.New("github", "awesome-copilot"),
				SkillName: "my-skill",
				Version:   "abc123def456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				// ResolveRef with explicit version tries branch first, then tag, then commit
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/ref/heads/abc123def456"),
					httpmock.StatusStringResponse(404, "not found"),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/ref/tags/abc123def456"),
					httpmock.StatusStringResponse(404, "not found"),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/commits/abc123def456"),
					httpmock.StringResponse(`{"sha": "abc123def456789012345678901234567890abcd"}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/trees/abc123def456789012345678901234567890abcd"),
					httpmock.StringResponse(`{
						"sha": "abc123def456789012345678901234567890abcd",
						"truncated": false,
						"tree": [
							{"path": "skills", "type": "tree", "sha": "tree1"},
							{"path": "skills/my-skill", "type": "tree", "sha": "treeSHA"},
							{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blob123"}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/trees/treeSHA"),
					httpmock.StringResponse(`{
						"tree": [
							{"path": "SKILL.md", "type": "blob", "sha": "blob123", "size": 50}
						]
					}`),
				)
				reg.Register(
					httpmock.REST("GET", "repos/github/awesome-copilot/git/blobs/blob123"),
					httpmock.StringResponse(`{"sha": "blob123", "content": "`+encodedContent+`", "encoding": "base64"}`),
				)
			},
			wantStdout: "My Skill",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			tt.opts.IO = ios

			tt.opts.Prompter = &prompter.PrompterMock{}
			tt.opts.Telemetry = &telemetry.NoOpService{}

			err := previewRun(tt.opts)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.wantStdout != "" {
				assert.Contains(t, stdout.String(), tt.wantStdout)
			}
		})
	}
}

func TestPreviewRun_UnsupportedHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	err := previewRun(&PreviewOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return &http.Client{}, nil },
		repo:       ghrepo.NewWithHost("github", "awesome-copilot", "acme.ghes.com"),
		Telemetry:  &telemetry.NoOpService{},
	})
	require.ErrorContains(t, err, "supports only github.com")
}

func TestPreviewRun_Interactive(t *testing.T) {
	skillContent := "# Selected Skill\n\nContent here."
	encodedContent := base64.StdEncoding.EncodeToString([]byte(skillContent))

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/releases/latest"),
		httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
		httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
		httpmock.StringResponse(`{
			"sha": "abc123",
			"truncated": false,
			"tree": [
				{"path": "skills/alpha", "type": "tree", "sha": "tree-a"},
				{"path": "skills/alpha/SKILL.md", "type": "blob", "sha": "blob-a"},
				{"path": "skills/beta", "type": "tree", "sha": "tree-b"},
				{"path": "skills/beta/SKILL.md", "type": "blob", "sha": "blob-b"}
			]
		}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/trees/tree-b"),
		httpmock.StringResponse(`{
			"tree": [
				{"path": "SKILL.md", "type": "blob", "sha": "blob-b", "size": 40}
			]
		}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/blobs/blob-b"),
		httpmock.StringResponse(`{"sha": "blob-b", "content": "`+encodedContent+`", "encoding": "base64"}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)

	pm := &prompter.PrompterMock{
		SelectFunc: func(prompt string, defaultValue string, options []string) (int, error) {
			assert.Equal(t, "Select a skill to preview:", prompt)
			assert.Equal(t, []string{"alpha", "beta"}, options)
			return 1, nil // select "beta"
		},
	}

	opts := &PreviewOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		Prompter:   pm,
		repo:       ghrepo.New("owner", "repo"),
		Telemetry:  &telemetry.NoOpService{},
	}

	err := previewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Selected Skill")
}

func TestPreviewRun_ShowsFileTree(t *testing.T) {
	skillContent := heredoc.Doc(`
		---
		name: my-skill
		description: test
		---
		# My Skill
		Body.
	`)
	encodedContent := base64.StdEncoding.EncodeToString([]byte(skillContent))

	scriptContent := "#!/bin/bash\necho hello"
	encodedScript := base64.StdEncoding.EncodeToString([]byte(scriptContent))

	makeReg := func() *httpmock.Registry {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/releases/latest"),
			httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
			httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
			httpmock.StringResponse(`{
				"sha": "abc123",
				"truncated": false,
				"tree": [
					{"path": "skills/my-skill", "type": "tree", "sha": "treeSHA"},
					{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blobSKILL"},
					{"path": "skills/my-skill/scripts", "type": "tree", "sha": "treeScripts"},
					{"path": "skills/my-skill/scripts/run.sh", "type": "blob", "sha": "blobScript"}
				]
			}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/trees/treeSHA"),
			httpmock.StringResponse(`{
				"tree": [
					{"path": "SKILL.md", "type": "blob", "sha": "blobSKILL", "size": 50},
					{"path": "scripts", "type": "tree", "sha": "treeScripts"},
					{"path": "scripts/run.sh", "type": "blob", "sha": "blobScript", "size": 20}
				]
			}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/blobs/blobSKILL"),
			httpmock.StringResponse(`{"sha": "blobSKILL", "content": "`+encodedContent+`", "encoding": "base64"}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/blobs/blobScript"),
			httpmock.StringResponse(`{"sha": "blobScript", "content": "`+encodedScript+`", "encoding": "base64"}`),
		)
		return reg
	}

	t.Run("interactive file picker", func(t *testing.T) {
		reg := makeReg()
		defer reg.Verify(t)
		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(true)
		ios.SetStdinTTY(true)
		ios.SetColorEnabled(false)

		selectCalls := 0
		pm := &prompter.PrompterMock{
			SelectFunc: func(prompt string, defaultValue string, options []string) (int, error) {
				selectCalls++
				if selectCalls == 1 {
					// Options: ["SKILL.md", "scripts/run.sh"]
					assert.Equal(t, "SKILL.md", options[0])
					assert.Equal(t, "scripts/run.sh", options[1])
					// Select "scripts/run.sh"
					return 1, nil
				}
				// Simulate Esc/Ctrl-C to exit
				return 0, fmt.Errorf("user cancelled")
			},
		}

		opts := &PreviewOptions{
			IO:         ios,
			HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
			Prompter:   pm,
			repo:       ghrepo.New("owner", "repo"),
			SkillName:  "my-skill",
			Telemetry:  &telemetry.NoOpService{},
		}

		err := previewRun(opts)
		require.NoError(t, err)

		out := stdout.String()
		assert.Contains(t, out, "echo hello")
		assert.Equal(t, 2, selectCalls)
	})

	t.Run("interactive markdown file uses markdown renderer", func(t *testing.T) {
		readmeContent := "# Usage\n\nUse **carefully**."
		encodedReadme := base64.StdEncoding.EncodeToString([]byte(readmeContent))

		reg := &httpmock.Registry{}
		defer reg.Verify(t)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/releases/latest"),
			httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
			httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
			httpmock.StringResponse(`{
				"sha": "abc123",
				"truncated": false,
				"tree": [
					{"path": "skills/my-skill", "type": "tree", "sha": "treeSHA"},
					{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blobSKILL"},
					{"path": "skills/my-skill/README.md", "type": "blob", "sha": "blobREADME"}
				]
			}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/trees/treeSHA"),
			httpmock.StringResponse(`{
				"tree": [
					{"path": "SKILL.md", "type": "blob", "sha": "blobSKILL", "size": 50},
					{"path": "README.md", "type": "blob", "sha": "blobREADME", "size": 28}
				]
			}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/blobs/blobSKILL"),
			httpmock.StringResponse(`{"sha": "blobSKILL", "content": "`+encodedContent+`", "encoding": "base64"}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/owner/repo/git/blobs/blobREADME"),
			httpmock.StringResponse(`{"sha": "blobREADME", "content": "`+encodedReadme+`", "encoding": "base64"}`),
		)

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(true)
		ios.SetStdinTTY(true)
		ios.SetColorEnabled(false)

		renderCalls := 0

		selectCalls := 0
		pm := &prompter.PrompterMock{
			SelectFunc: func(prompt string, defaultValue string, options []string) (int, error) {
				selectCalls++
				if selectCalls == 1 {
					assert.Equal(t, []string{"SKILL.md", "README.md"}, options)
					return 1, nil
				}
				return 0, fmt.Errorf("user cancelled")
			},
		}

		opts := &PreviewOptions{
			IO:         ios,
			HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
			Prompter:   pm,
			repo:       ghrepo.New("owner", "repo"),
			SkillName:  "my-skill",
			RenderFile: func(filePath, content string) string {
				renderCalls++
				return fmt.Sprintf("rendered:%s", filePath)
			},
			Telemetry: &telemetry.NoOpService{},
		}

		err := previewRun(opts)
		require.NoError(t, err)

		out := stdout.String()
		assert.Contains(t, out, "rendered:README.md")
		assert.Equal(t, 2, selectCalls)
		assert.Equal(t, 2, renderCalls)
	})

	t.Run("non-interactive dumps all files", func(t *testing.T) {
		reg := makeReg()
		defer reg.Verify(t)
		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(false)
		ios.SetStdinTTY(false)
		ios.SetColorEnabled(false)

		opts := &PreviewOptions{
			IO:         ios,
			HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
			Prompter:   &prompter.PrompterMock{},
			repo:       ghrepo.New("owner", "repo"),
			SkillName:  "my-skill",
			Telemetry:  &telemetry.NoOpService{},
		}

		err := previewRun(opts)
		require.NoError(t, err)

		out := stdout.String()
		assert.Contains(t, out, "my-skill/")
		assert.Contains(t, out, "My Skill")
		assert.Contains(t, out, "scripts/run.sh")
		assert.Contains(t, out, "echo hello")
	})
}

func TestPreviewRun_RenderLimits(t *testing.T) {
	skillContent := heredoc.Doc(`
		---
		name: my-skill
		description: test
		---
		# My Skill
	`)
	encodedSkill := base64.StdEncoding.EncodeToString([]byte(skillContent))

	// Helper: build a tree JSON with N extra files (beyond SKILL.md)
	buildTree := func(n int) string {
		entries := []string{
			`{"path": "skills/my-skill", "type": "tree", "sha": "treeSHA"}`,
			`{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blobSKILL"}`,
		}
		for i := range n {
			entries = append(entries, fmt.Sprintf(
				`{"path": "skills/my-skill/file%03d.txt", "type": "blob", "sha": "blob%03d"}`, i, i))
		}
		return fmt.Sprintf(`{"sha":"abc123","truncated":false,"tree":[%s]}`,
			strings.Join(entries, ","))
	}

	// Helper: build subtree JSON with N extra files
	buildSubtree := func(n int, sizes []int) string {
		entries := []string{
			`{"path": "SKILL.md", "type": "blob", "sha": "blobSKILL", "size": 50}`,
		}
		for i := range n {
			sz := 10
			if i < len(sizes) {
				sz = sizes[i]
			}
			entries = append(entries, fmt.Sprintf(
				`{"path": "file%03d.txt", "type": "blob", "sha": "blob%03d", "size": %d}`, i, i, sz))
		}
		return fmt.Sprintf(`{"tree":[%s]}`, strings.Join(entries, ","))
	}

	// Common stubs for resolve + discover
	registerBase := func(reg *httpmock.Registry, treeJSON, subtreeJSON string) {
		reg.Register(
			httpmock.REST("GET", "repos/monalisa/skills-repo/releases/latest"),
			httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/monalisa/skills-repo/git/ref/tags/v1.0.0"),
			httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
		)
		reg.Register(
			httpmock.REST("GET", "repos/monalisa/skills-repo/git/trees/abc123"),
			httpmock.StringResponse(treeJSON),
		)
		reg.Register(
			httpmock.REST("GET", "repos/monalisa/skills-repo/git/trees/treeSHA"),
			httpmock.StringResponse(subtreeJSON),
		)
		reg.Register(
			httpmock.REST("GET", "repos/monalisa/skills-repo/git/blobs/blobSKILL"),
			httpmock.StringResponse(`{"sha": "blobSKILL", "content": "`+encodedSkill+`", "encoding": "base64"}`),
		)
	}

	t.Run("maxFiles cap truncates at 20", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)

		n := 22
		treeJSON := buildTree(n)
		subtreeJSON := buildSubtree(n, nil)
		registerBase(reg, treeJSON, subtreeJSON)

		// Register blob stubs for files 0-19 (first 20 get fetched)
		tinyContent := base64.StdEncoding.EncodeToString([]byte("tiny"))
		for i := range 20 {
			reg.Register(
				httpmock.REST("GET", fmt.Sprintf("repos/monalisa/skills-repo/git/blobs/blob%03d", i)),
				httpmock.StringResponse(fmt.Sprintf(`{"sha": "blob%03d", "content": "%s", "encoding": "base64"}`, i, tinyContent)),
			)
		}
		// Files 20 and 21 should NOT be fetched

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(false)
		ios.SetStdinTTY(false)

		opts := &PreviewOptions{
			IO:         ios,
			HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
			Prompter:   &prompter.PrompterMock{},
			repo:       ghrepo.New("monalisa", "skills-repo"),
			SkillName:  "my-skill",
			Telemetry:  &telemetry.NoOpService{},
		}

		err := previewRun(opts)
		require.NoError(t, err)

		out := stdout.String()
		assert.Contains(t, out, "showing first 20")
		assert.Contains(t, out, "file019.txt") // last fetched
	})

	t.Run("maxBytes cap stops fetching", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)

		// Two files: first is 500KB, second would exceed 512KB cap
		sizes := []int{500 * 1024, 100 * 1024}
		treeJSON := buildTree(2)
		subtreeJSON := buildSubtree(2, sizes)
		registerBase(reg, treeJSON, subtreeJSON)

		bigContent := base64.StdEncoding.EncodeToString(make([]byte, 500*1024))
		reg.Register(
			httpmock.REST("GET", "repos/monalisa/skills-repo/git/blobs/blob000"),
			httpmock.StringResponse(fmt.Sprintf(`{"sha": "blob000", "content": "%s", "encoding": "base64"}`, bigContent)),
		)
		// blob001 should NOT be fetched (size limit reached)

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(false)
		ios.SetStdinTTY(false)

		opts := &PreviewOptions{
			IO:         ios,
			HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
			Prompter:   &prompter.PrompterMock{},
			repo:       ghrepo.New("monalisa", "skills-repo"),
			SkillName:  "my-skill",
			Telemetry:  &telemetry.NoOpService{},
		}

		err := previewRun(opts)
		require.NoError(t, err)

		out := stdout.String()
		assert.Contains(t, out, "size limit reached")
	})

	t.Run("blob fetch error shows fallback message", func(t *testing.T) {
		reg := &httpmock.Registry{}
		defer reg.Verify(t)

		treeJSON := buildTree(1)
		subtreeJSON := buildSubtree(1, nil)
		registerBase(reg, treeJSON, subtreeJSON)

		reg.Register(
			httpmock.REST("GET", "repos/monalisa/skills-repo/git/blobs/blob000"),
			httpmock.StatusStringResponse(500, "server error"),
		)

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(false)
		ios.SetStdinTTY(false)

		opts := &PreviewOptions{
			IO:         ios,
			HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
			Prompter:   &prompter.PrompterMock{},
			repo:       ghrepo.New("monalisa", "skills-repo"),
			SkillName:  "my-skill",
			Telemetry:  &telemetry.NoOpService{},
		}

		err := previewRun(opts)
		require.NoError(t, err)

		out := stdout.String()
		assert.Contains(t, out, "could not fetch file")
	})
}

func TestPreviewRun_InteractiveTelemetryCapturesSelectedSkillName(t *testing.T) {
	skillContent := "# Selected Skill\n\nContent here."
	encodedContent := base64.StdEncoding.EncodeToString([]byte(skillContent))

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/releases/latest"),
		httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
		httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
		httpmock.StringResponse(`{
			"sha": "abc123",
			"truncated": false,
			"tree": [
				{"path": "skills/alpha", "type": "tree", "sha": "tree-a"},
				{"path": "skills/alpha/SKILL.md", "type": "blob", "sha": "blob-a"},
				{"path": "skills/beta", "type": "tree", "sha": "tree-b"},
				{"path": "skills/beta/SKILL.md", "type": "blob", "sha": "blob-b"}
			]
		}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/trees/tree-b"),
		httpmock.StringResponse(`{
			"tree": [
				{"path": "SKILL.md", "type": "blob", "sha": "blob-b", "size": 40}
			]
		}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo/git/blobs/blob-b"),
		httpmock.StringResponse(`{"sha": "blob-b", "content": "`+encodedContent+`", "encoding": "base64"}`),
	)
	reg.Register(
		httpmock.REST("GET", "repos/owner/repo"),
		httpmock.JSONResponse(map[string]interface{}{
			"visibility": "public",
		}),
	)

	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)

	pm := &prompter.PrompterMock{
		SelectFunc: func(prompt string, defaultValue string, options []string) (int, error) {
			return 1, nil // select "beta"
		},
	}

	recorder := &telemetry.EventRecorderSpy{}

	opts := &PreviewOptions{
		IO:         ios,
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		Prompter:   pm,
		Telemetry:  recorder,
		repo:       ghrepo.New("owner", "repo"),
		// SkillName intentionally left empty to simulate interactive selection
	}

	err := previewRun(opts)
	require.NoError(t, err)

	// Verify the telemetry event captured the interactively-selected skill name, not empty string
	require.Len(t, recorder.Events, 1)
	event := recorder.Events[0]
	assert.Equal(t, "skill_preview", event.Type)
	assert.Equal(t, "beta", event.Dimensions["skill_name"], "telemetry should capture the selected skill name, not the empty opts.SkillName")
}

func TestPreviewRun_TelemetryVisibility(t *testing.T) {
	skillContent := heredoc.Doc(`
		---
		name: my-skill
		description: test
		---
		# My Skill
		Body.
	`)
	encodedContent := base64.StdEncoding.EncodeToString([]byte(skillContent))

	tests := []struct {
		name           string
		visibility     string
		visibilityErr  bool
		wantSkillNames string
	}{
		{
			name:           "public repo includes skill names",
			visibility:     "public",
			wantSkillNames: "my-skill",
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

			reg.Register(
				httpmock.REST("GET", "repos/owner/repo/releases/latest"),
				httpmock.StringResponse(`{"tag_name": "v1.0.0"}`),
			)
			reg.Register(
				httpmock.REST("GET", "repos/owner/repo/git/ref/tags/v1.0.0"),
				httpmock.StringResponse(`{"object": {"sha": "abc123", "type": "commit"}}`),
			)
			reg.Register(
				httpmock.REST("GET", "repos/owner/repo/git/trees/abc123"),
				httpmock.StringResponse(`{
					"sha": "abc123",
					"truncated": false,
					"tree": [
						{"path": "skills/my-skill", "type": "tree", "sha": "treeSHA"},
						{"path": "skills/my-skill/SKILL.md", "type": "blob", "sha": "blobSKILL"}
					]
				}`),
			)
			reg.Register(
				httpmock.REST("GET", "repos/owner/repo/git/trees/treeSHA"),
				httpmock.StringResponse(`{
					"tree": [
						{"path": "SKILL.md", "type": "blob", "sha": "blobSKILL", "size": 50}
					]
				}`),
			)
			reg.Register(
				httpmock.REST("GET", "repos/owner/repo/git/blobs/blobSKILL"),
				httpmock.StringResponse(`{"sha": "blobSKILL", "content": "`+encodedContent+`", "encoding": "base64"}`),
			)
			if tt.visibilityErr {
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo"),
					httpmock.StatusStringResponse(500, "server error"),
				)
			} else {
				reg.Register(
					httpmock.REST("GET", "repos/owner/repo"),
					httpmock.JSONResponse(map[string]interface{}{
						"visibility": tt.visibility,
					}),
				)
			}

			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(false)
			ios.SetStdinTTY(false)

			recorder := &telemetry.EventRecorderSpy{}

			opts := &PreviewOptions{
				IO:         ios,
				HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
				Prompter:   &prompter.PrompterMock{},
				Telemetry:  recorder,
				repo:       ghrepo.New("owner", "repo"),
				SkillName:  "my-skill",
			}

			err := previewRun(opts)
			require.NoError(t, err)

			require.Len(t, recorder.Events, 1)
			event := recorder.Events[0]
			assert.Equal(t, "skill_preview", event.Type)

			// skill_host_type is always recorded (categorized, no raw hostname for enterprise/tenancy).
			assert.Equal(t, "github.com", event.Dimensions["skill_host_type"])

			if tt.visibilityErr {
				assert.Equal(t, "unknown", event.Dimensions["repo_visibility"],
					"visibility fetch errors should emit repo_visibility=\"unknown\" so the fallback is distinguishable from a successful fetch")
			} else {
				assert.Equal(t, tt.visibility, event.Dimensions["repo_visibility"])
			}

			// Owner, repo, and skill name are only included when the repo
			// is public; for private/internal/unknown they are omitted to
			// avoid leaking identifiers of non-public repositories.
			if tt.wantSkillNames != "" {
				assert.Equal(t, "owner", event.Dimensions["skill_owner"])
				assert.Equal(t, "repo", event.Dimensions["skill_repo"])
				assert.Equal(t, tt.wantSkillNames, event.Dimensions["skill_name"])
			} else {
				assert.Empty(t, event.Dimensions["skill_owner"])
				assert.Empty(t, event.Dimensions["skill_repo"])
				assert.Empty(t, event.Dimensions["skill_name"])
			}
		})
	}
}
