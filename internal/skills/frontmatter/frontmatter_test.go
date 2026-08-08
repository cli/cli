package frontmatter

import (
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantDesc string
		wantBody string
		wantErr  bool
	}{
		{
			name: "valid frontmatter",
			content: heredoc.Doc(`
				---
				name: test-skill
				description: A test skill
				---
				# Body
			`),
			wantName: "test-skill",
			wantDesc: "A test skill",
			wantBody: "# Body\n",
		},
		{
			name:     "no frontmatter",
			content:  "# Just a markdown file\n",
			wantBody: "# Just a markdown file\n",
		},
		{
			name:    "invalid YAML",
			content: "---\n: invalid yaml [[\n---\n",
			wantErr: true,
		},
		{
			name:     "no closing delimiter",
			content:  "---\nname: test\n",
			wantBody: "---\nname: test\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(tt.content)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, result.Metadata.Name)
			assert.Equal(t, tt.wantDesc, result.Metadata.Description)
			assert.Equal(t, tt.wantBody, result.Body)
		})
	}
}

func TestInjectGitHubMetadata(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		host           string
		owner          string
		repo           string
		ref            string
		treeSHA        string
		pinnedRef      string
		skillPath      string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "injects metadata without pin",
			content: heredoc.Doc(`
				---
				name: my-skill
				description: desc
				---
				# Body
			`),
			host:      "github.com",
			owner:     "monalisa",
			repo:      "octocat-skills",
			ref:       "refs/tags/v1.0.0",
			treeSHA:   "tree456",
			pinnedRef: "",
			skillPath: "skills/my-skill",
			wantContains: []string{
				"github-repo: https://github.com/monalisa/octocat-skills",
				"github-ref: refs/tags/v1.0.0",
				"github-tree-sha: tree456",
				"github-path: skills/my-skill",
				"# Body",
			},
			wantNotContain: []string{
				"github-owner",
				"github-sha",
				"github-pinned",
			},
		},
		{
			name: "injects pinned ref",
			content: heredoc.Doc(`
				---
				name: my-skill
				---
				# Body
			`),
			host:      "github.com",
			owner:     "monalisa",
			repo:      "octocat-skills",
			ref:       "refs/tags/v1.0.0",
			treeSHA:   "tree",
			pinnedRef: "v1.0.0",
			skillPath: "skills/my-skill",
			wantContains: []string{
				"github-pinned: v1.0.0",
			},
		},
		{
			name:      "injects metadata into content with no frontmatter",
			content:   "# Body only\n",
			host:      "github.com",
			owner:     "monalisa",
			repo:      "octocat-skills",
			ref:       "refs/heads/main",
			treeSHA:   "tree456",
			pinnedRef: "",
			skillPath: "skills/my-skill",
			wantContains: []string{
				"github-repo: https://github.com/monalisa/octocat-skills",
				"github-ref: refs/heads/main",
				"# Body only",
			},
			wantNotContain: []string{"github-owner", "github-sha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InjectGitHubMetadata(tt.content, tt.host, tt.owner, tt.repo, tt.ref, tt.treeSHA, tt.pinnedRef, tt.skillPath)
			require.NoError(t, err)
			for _, s := range tt.wantContains {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.wantNotContain {
				assert.NotContains(t, got, s)
			}
		})
	}
}

func TestInjectLocalMetadata(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "strips all github keys and injects local-path",
			content: heredoc.Doc(`
				---
				name: my-skill
				metadata:
				    github-owner: old
				    github-repo: old
				    github-ref: v1.0.0
				    github-sha: abc123
				    github-tree-sha: tree456
				    github-pinned: v1.0.0
				    github-path: skills/my-skill
				---
				# Body
			`),
			wantContains:   []string{"local-path: /home/monalisa/skills/my-skill"},
			wantNotContain: []string{"github-owner", "github-repo", "github-ref", "github-sha", "github-tree-sha", "github-pinned", "github-path"},
		},
		{
			name:         "injects into content with no existing metadata",
			content:      "# Body only\n",
			wantContains: []string{"local-path: /home/monalisa/skills/my-skill"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InjectLocalMetadata(tt.content, "/home/monalisa/skills/my-skill")
			require.NoError(t, err)
			for _, s := range tt.wantContains {
				assert.Contains(t, got, s)
			}
			for _, s := range tt.wantNotContain {
				assert.NotContains(t, got, s)
			}
		})
	}
}

func TestSerialize(t *testing.T) {
	tests := []struct {
		name         string
		frontmatter  map[string]interface{}
		body         string
		wantPrefix   string
		wantSuffix   string
		wantContains []string
	}{
		{
			name:        "with body",
			frontmatter: map[string]interface{}{"name": "test"},
			body:        "# Body content",
			wantPrefix:  "---\n",
			wantContains: []string{
				"name: test",
				"# Body content",
			},
		},
		{
			name:        "empty body",
			frontmatter: map[string]interface{}{"name": "test"},
			body:        "",
			wantSuffix:  "---\n",
		},
		{
			name:        "body without trailing newline gets one added",
			frontmatter: map[string]interface{}{"name": "test"},
			body:        "# No trailing newline",
			wantSuffix:  "# No trailing newline\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Serialize(tt.frontmatter, tt.body)
			require.NoError(t, err)
			if tt.wantPrefix != "" {
				assert.True(t, strings.HasPrefix(got, tt.wantPrefix))
			}
			if tt.wantSuffix != "" {
				assert.True(t, strings.HasSuffix(got, tt.wantSuffix))
			}
			for _, s := range tt.wantContains {
				assert.Contains(t, got, s)
			}
		})
	}
}
