package discovery

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallName(t *testing.T) {
	tests := []struct {
		name     string
		skill    Skill
		wantName string
	}{
		{
			name:     "plain skill",
			skill:    Skill{Name: "code-review"},
			wantName: "code-review",
		},
		{
			name:     "namespaced skill",
			skill:    Skill{Name: "issue-triage", Namespace: "monalisa"},
			wantName: "monalisa/issue-triage",
		},
		{
			name:     "plugin skill with namespace",
			skill:    Skill{Name: "pr-summary", Namespace: "hubot", Convention: "plugins"},
			wantName: "hubot/pr-summary",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.skill.InstallName())
		})
	}
}

func TestMatchSkillConventions(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		wantNil        bool
		wantName       string
		wantNamespace  string
		wantConvention string
	}{
		{
			name:           "plugin namespace",
			path:           "plugins/hubot/skills/pr-summary/SKILL.md",
			wantName:       "pr-summary",
			wantNamespace:  "hubot",
			wantConvention: "plugins",
		},
		{
			name:           "namespaced skill",
			path:           "skills/monalisa/issue-triage/SKILL.md",
			wantName:       "issue-triage",
			wantNamespace:  "monalisa",
			wantConvention: "skills-namespaced",
		},
		{
			name:           "regular skill",
			path:           "skills/code-review/SKILL.md",
			wantName:       "code-review",
			wantConvention: "skills",
		},
		{
			name:    "non-SKILL.md file",
			path:    "skills/code-review/README.md",
			wantNil: true,
		},
		{
			name:           "plugin skill from different author",
			path:           "plugins/monalisa/skills/code-review/SKILL.md",
			wantName:       "code-review",
			wantNamespace:  "monalisa",
			wantConvention: "plugins",
		},
		{
			name:           "root convention single-skill repo",
			path:           "code-review/SKILL.md",
			wantName:       "code-review",
			wantConvention: "root",
		},
		{
			name:    "root convention excludes skills dir",
			path:    "skills/SKILL.md",
			wantNil: true,
		},
		{
			name:    "root convention excludes dot-prefixed",
			path:    ".hidden/SKILL.md",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matchSkillConventions(treeEntry{Path: tt.path, Type: "blob"})
			if tt.wantNil {
				assert.Nil(t, m)
				return
			}
			require.NotNil(t, m)
			assert.Equal(t, tt.wantName, m.name)
			assert.Equal(t, tt.wantNamespace, m.namespace)
			assert.Equal(t, tt.wantConvention, m.convention)
		})
	}
}

func TestMatchHiddenDirConventions(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		wantNil        bool
		wantName       string
		wantNamespace  string
		wantConvention string
	}{
		{
			name:           "claude skills directory",
			path:           ".claude/skills/code-review/SKILL.md",
			wantName:       "code-review",
			wantConvention: "hidden-dir",
		},
		{
			name:           "agents skills directory",
			path:           ".agents/skills/git-commit/SKILL.md",
			wantName:       "git-commit",
			wantConvention: "hidden-dir",
		},
		{
			name:           "github skills directory",
			path:           ".github/skills/issue-triage/SKILL.md",
			wantName:       "issue-triage",
			wantConvention: "hidden-dir",
		},
		{
			name:           "copilot skills directory",
			path:           ".copilot/skills/pr-summary/SKILL.md",
			wantName:       "pr-summary",
			wantConvention: "hidden-dir",
		},
		{
			name:           "namespaced hidden dir skill",
			path:           ".claude/skills/monalisa/code-review/SKILL.md",
			wantName:       "code-review",
			wantNamespace:  "monalisa",
			wantConvention: "hidden-dir-namespaced",
		},
		{
			name:    "not a SKILL.md file",
			path:    ".claude/skills/code-review/README.md",
			wantNil: true,
		},
		{
			name:    "too shallow - just hidden dir and SKILL.md",
			path:    ".claude/SKILL.md",
			wantNil: true,
		},
		{
			name:    "no skills subdirectory",
			path:    ".claude/code-review/SKILL.md",
			wantNil: true,
		},
		{
			name:    "non-hidden dir does not match",
			path:    "visible/skills/code-review/SKILL.md",
			wantNil: true,
		},
		{
			name:    "non-hidden-namespaced dir does not match",
			path:    "visible/skills/monalisa/code-review/SKILL.md",
			wantNil: true,
		},
		{
			name:    "too deeply nested hidden dir",
			path:    ".claude/nested/skills/code-review/SKILL.md",
			wantNil: true,
		},
		{
			name:    "invalid skill name",
			path:    ".claude/skills/../SKILL.md",
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := matchHiddenDirConventions(treeEntry{Path: tt.path, Type: "blob"})
			if tt.wantNil {
				assert.Nil(t, m)
				return
			}
			require.NotNil(t, m)
			assert.Equal(t, tt.wantName, m.name)
			assert.Equal(t, tt.wantNamespace, m.namespace)
			assert.Equal(t, tt.wantConvention, m.convention)
		})
	}
}

func TestHasHiddenDirSkills(t *testing.T) {
	tests := []struct {
		name   string
		skills []Skill
		want   bool
	}{
		{
			name:   "empty list",
			skills: nil,
			want:   false,
		},
		{
			name:   "only standard skills",
			skills: []Skill{{Convention: "skills"}, {Convention: "root"}},
			want:   false,
		},
		{
			name:   "has hidden-dir skill",
			skills: []Skill{{Convention: "skills"}, {Convention: "hidden-dir"}},
			want:   true,
		},
		{
			name:   "has hidden-dir-namespaced skill",
			skills: []Skill{{Convention: "hidden-dir-namespaced"}},
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasHiddenDirSkills(tt.skills))
		})
	}
}

func TestDisplayNameHiddenDir(t *testing.T) {
	tests := []struct {
		name     string
		skill    Skill
		wantName string
	}{
		{
			name:     "hidden-dir skill",
			skill:    Skill{Name: "code-review", Convention: "hidden-dir"},
			wantName: "[hidden-dir] code-review",
		},
		{
			name:     "hidden-dir-namespaced skill",
			skill:    Skill{Name: "code-review", Namespace: "monalisa", Convention: "hidden-dir-namespaced"},
			wantName: "[hidden-dir] monalisa/code-review",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, tt.skill.DisplayName())
		})
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty", input: "", want: false},
		{name: "too long", input: strings.Repeat("a", 65), want: false},
		{name: "max length is valid", input: strings.Repeat("a", 64), want: true},
		{name: "contains slash", input: "foo/bar", want: false},
		{name: "contains dotdot", input: "foo..bar", want: false},
		{name: "starts with dot", input: ".hidden", want: false},
		{name: "simple name", input: "code-review", want: true},
		{name: "with dots and underscores", input: "octocat_helper.v2", want: true},
		{name: "uppercase allowed", input: "Octocat", want: true},
		{name: "single char", input: "a", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, validateName(tt.input))
		})
	}
}

func TestIsSpecCompliant(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty", input: "", want: false},
		{name: "consecutive hyphens", input: "code--review", want: false},
		{name: "uppercase rejected", input: "Octocat", want: false},
		{name: "starts with hyphen", input: "-octocat", want: false},
		{name: "ends with hyphen", input: "octocat-", want: false},
		{name: "valid lowercase with hyphens", input: "issue-triage", want: true},
		{name: "valid single char", input: "a", want: true},
		{name: "valid with numbers", input: "copilot4", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsSpecCompliant(tt.input))
		})
	}
}

func TestIsFullyQualifiedRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{name: "branch ref", ref: "refs/heads/main", want: true},
		{name: "tag ref", ref: "refs/tags/v1.0", want: true},
		{name: "short branch name", ref: "main", want: false},
		{name: "short tag name", ref: "v1.0", want: false},
		{name: "bare SHA", ref: "abc123def456", want: false},
		{name: "empty", ref: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsFullyQualifiedRef(tt.ref))
		})
	}
}

func TestShortRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "branch ref", ref: "refs/heads/main", want: "main"},
		{name: "tag ref", ref: "refs/tags/v1.0", want: "v1.0"},
		{name: "short name passthrough", ref: "main", want: "main"},
		{name: "bare SHA passthrough", ref: "abc123", want: "abc123"},
		{name: "empty passthrough", ref: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShortRef(tt.ref))
		})
	}
}

func TestResolveRef(t *testing.T) {
	tests := []struct {
		name    string
		version string
		stubs   func(*httpmock.Registry)
		wantRef string
		wantSHA string
		wantErr string
	}{
		{
			name:    "short name resolves as branch first",
			version: "main",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/main"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "branch-sha"},
					}))
			},
			wantRef: "refs/heads/main",
			wantSHA: "branch-sha",
		},
		{
			name:    "short name falls back to tag when branch not found",
			version: "v1.0",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/v1.0"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v1.0"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "abc123", "type": "commit"},
					}))
			},
			wantRef: "refs/tags/v1.0",
			wantSHA: "abc123",
		},
		{
			name:    "short name resolves annotated tag",
			version: "v2.0",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/v2.0"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v2.0"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "tag-obj-sha", "type": "tag"},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/tags/tag-obj-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "real-commit-sha"},
					}))
			},
			wantRef: "refs/tags/v2.0",
			wantSHA: "real-commit-sha",
		},
		{
			name:    "short name falls back to commit SHA",
			version: "deadbeef",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/deadbeef"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/deadbeef"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/commits/deadbeef"),
					httpmock.JSONResponse(map[string]interface{}{"sha": "deadbeef"}))
			},
			wantRef: "deadbeef",
			wantSHA: "deadbeef",
		},
		{
			name:    "short name not found anywhere",
			version: "nonexistent",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/nonexistent"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/nonexistent"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/commits/nonexistent"),
					httpmock.StatusStringResponse(404, "not found"))
			},
			wantErr: `ref "nonexistent" not found as branch, tag, or commit in monalisa/octocat-skills`,
		},
		{
			name:    "branch wins over tag with same short name",
			version: "release",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/release"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "branch-sha"},
					}))
				// tag stub is not registered because branch succeeds first
			},
			wantRef: "refs/heads/release",
			wantSHA: "branch-sha",
		},
		{
			name:    "fully qualified tag ref resolved directly",
			version: "refs/tags/v1.0",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v1.0"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "tag-sha", "type": "commit"},
					}))
			},
			wantRef: "refs/tags/v1.0",
			wantSHA: "tag-sha",
		},
		{
			name:    "fully qualified branch ref resolved directly",
			version: "refs/heads/feature",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/feature"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "feature-sha"},
					}))
			},
			wantRef: "refs/heads/feature",
			wantSHA: "feature-sha",
		},
		{
			name:    "fully qualified tag ref not found",
			version: "refs/tags/nonexistent",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/nonexistent"),
					httpmock.StatusStringResponse(404, "not found"))
			},
			wantErr: `tag "nonexistent" not found in monalisa/octocat-skills`,
		},
		{
			name:    "fully qualified branch ref not found",
			version: "refs/heads/nonexistent",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/nonexistent"),
					httpmock.StatusStringResponse(404, "not found"))
			},
			wantErr: `branch "nonexistent" not found in monalisa/octocat-skills`,
		},
		{
			name: "no version uses latest release with fully qualified ref",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.JSONResponse(map[string]interface{}{"tag_name": "v3.0"}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v3.0"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "release-sha", "type": "commit"},
					}))
			},
			wantRef: "refs/tags/v3.0",
			wantSHA: "release-sha",
		},
		{
			name: "no version falls back to default branch with fully qualified ref",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/main"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "branch-sha"},
					}))
			},
			wantRef: "refs/heads/main",
			wantSHA: "branch-sha",
		},
		{
			name:    "annotated tag dereference failure",
			version: "refs/tags/v4.0",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v4.0"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "tag-obj-sha", "type": "tag"},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/tags/tag-obj-sha"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: "could not dereference annotated tag",
		},
		{
			name: "no version with server error does not fall back to default branch",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StatusStringResponse(500, "internal server error"))
			},
			wantErr: "could not fetch latest release",
		},
		{
			name: "no version with forbidden error does not fall back to default branch",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StatusStringResponse(403, "forbidden"))
			},
			wantErr: "could not fetch latest release",
		},
		{
			name: "empty tag_name in latest release falls back to default branch",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.JSONResponse(map[string]interface{}{"tag_name": ""}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": "main"}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/main"),
					httpmock.JSONResponse(map[string]interface{}{
						"object": map[string]interface{}{"sha": "fallback-sha"},
					}))
			},
			wantRef: "refs/heads/main",
			wantSHA: "fallback-sha",
		},
		{
			name: "empty default_branch returns error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/releases/latest"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{"default_branch": ""}))
			},
			wantErr: "could not determine default branch",
		},
		{
			name:    "short name with server error on branch lookup does not fall through",
			version: "main",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/main"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: `branch "main" not found in monalisa/octocat-skills`,
		},
		{
			name:    "short name with forbidden error on branch lookup does not fall through",
			version: "develop",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/develop"),
					httpmock.StatusStringResponse(403, "forbidden"))
			},
			wantErr: `branch "develop" not found in monalisa/octocat-skills`,
		},
		{
			name:    "short name with server error on tag lookup does not fall through",
			version: "v5.0",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/heads/v5.0"),
					httpmock.StatusStringResponse(404, "not found"))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/ref/tags/v5.0"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: `tag "v5.0" not found in monalisa/octocat-skills`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			ref, err := ResolveRef(client, "github.com", "monalisa", "octocat-skills", tt.version)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantRef, ref.Ref)
			assert.Equal(t, tt.wantSHA, ref.SHA)
		})
	}
}

func TestFetchBlob(t *testing.T) {
	tests := []struct {
		name    string
		stubs   func(*httpmock.Registry)
		wantErr string
		want    string
	}{
		{
			name: "decodes base64 content",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/abc"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "abc", "encoding": "base64", "content": "SGVsbG8gV29ybGQ=",
					}))
			},
			want: "Hello World",
		},
		{
			name: "rejects non-base64 encoding",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/abc"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "abc", "encoding": "utf-8", "content": "raw",
					}))
			},
			wantErr: "unexpected blob encoding: utf-8",
		},
		{
			name: "API error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/abc"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: "could not fetch blob",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			got, err := FetchBlob(client, "github.com", "monalisa", "octocat-skills", "abc")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFetchRepoVisibility(t *testing.T) {
	tests := []struct {
		name    string
		stubs   func(*httpmock.Registry)
		want    RepoVisibility
		wantErr string
	}{
		{
			name: "public repo",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{
						"visibility": "public",
					}))
			},
			want: RepoVisibilityPublic,
		},
		{
			name: "private repo",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{
						"visibility": "private",
					}))
			},
			want: RepoVisibilityPrivate,
		},
		{
			name: "internal repo",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{
						"visibility": "internal",
					}))
			},
			want: RepoVisibilityInternal,
		},
		{
			name: "unknown visibility",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.JSONResponse(map[string]interface{}{
						"visibility": "cool-visibility",
					}))
			},
			wantErr: `unknown repository visibility: "cool-visibility"`,
		},
		{
			name: "API error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: "HTTP 500",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			got, err := FetchRepoVisibility(client, "github.com", "monalisa", "octocat-skills")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDiscoverSkills(t *testing.T) {
	tests := []struct {
		name       string
		stubs      func(*httpmock.Registry)
		wantSkills []string
		wantErr    string
	}{
		{
			name: "discovers skills from tree",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/abc123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "abc123", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "skills/code-review", "type": "tree", "sha": "tree-sha-1"},
							{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "blob-1"},
							{"path": "skills/issue-triage", "type": "tree", "sha": "tree-sha-2"},
							{"path": "skills/issue-triage/SKILL.md", "type": "blob", "sha": "blob-2"},
							{"path": "README.md", "type": "blob", "sha": "readme"},
						},
					}))
			},
			wantSkills: []string{"code-review", "issue-triage"},
		},
		{
			name: "truncated tree returns error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/abc123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "abc123", "truncated": true, "tree": []map[string]interface{}{},
					}))
			},
			wantErr: "too large",
		},
		{
			name: "no skills found",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/abc123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "abc123", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "README.md", "type": "blob", "sha": "readme"},
						},
					}))
			},
			wantErr: "no skills found",
		},
		{
			name: "API error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/abc123"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: "could not fetch repository tree",
		},
		{
			name: "deduplicates skills from same directory",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/abc123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "abc123", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "skills/code-review", "type": "tree", "sha": "tree-sha"},
							{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "blob-1"},
							{"path": "skills/code-review/SKILL.md", "type": "blob", "sha": "blob-2"},
						},
					}))
			},
			wantSkills: []string{"code-review"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			skills, err := DiscoverSkills(client, "github.com", "monalisa", "octocat-skills", "abc123")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var names []string
			for _, s := range skills {
				names = append(names, s.Name)
			}
			assert.Equal(t, tt.wantSkills, names)
		})
	}
}

func TestDiscoverSkillsWithOptions(t *testing.T) {
	hiddenDirTree := map[string]interface{}{
		"sha": "abc123", "truncated": false,
		"tree": []map[string]interface{}{
			{"path": ".claude/skills/code-review", "type": "tree", "sha": "tree-sha-1"},
			{"path": ".claude/skills/code-review/SKILL.md", "type": "blob", "sha": "blob-1"},
			{"path": ".agents/skills/git-commit", "type": "tree", "sha": "tree-sha-2"},
			{"path": ".agents/skills/git-commit/SKILL.md", "type": "blob", "sha": "blob-2"},
			{"path": "README.md", "type": "blob", "sha": "readme"},
		},
	}

	mixedTree := map[string]interface{}{
		"sha": "abc123", "truncated": false,
		"tree": []map[string]interface{}{
			{"path": "skills/standard-skill", "type": "tree", "sha": "tree-sha-1"},
			{"path": "skills/standard-skill/SKILL.md", "type": "blob", "sha": "blob-1"},
			{"path": ".claude/skills/hidden-skill", "type": "tree", "sha": "tree-sha-2"},
			{"path": ".claude/skills/hidden-skill/SKILL.md", "type": "blob", "sha": "blob-2"},
		},
	}

	emptyTree := map[string]interface{}{
		"sha": "abc123", "truncated": false,
		"tree": []map[string]interface{}{
			{"path": "README.md", "type": "blob", "sha": "readme"},
		},
	}

	tests := []struct {
		name       string
		tree       map[string]interface{}
		wantSkills []string
		wantErr    string
	}{
		{
			name:       "returns hidden-dir skills",
			tree:       hiddenDirTree,
			wantSkills: []string{"code-review", "git-commit"},
		},
		{
			name:       "mixed tree returns all skills",
			tree:       mixedTree,
			wantSkills: []string{"hidden-skill", "standard-skill"},
		},
		{
			name:    "no skills at all",
			tree:    emptyTree,
			wantErr: "no skills found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/abc123"),
				httpmock.JSONResponse(tt.tree))
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			skills, err := DiscoverSkillsWithOptions(client, "github.com", "monalisa", "octocat-skills", "abc123", DiscoverOptions{})
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var names []string
			for _, s := range skills {
				names = append(names, s.Name)
			}
			assert.Equal(t, tt.wantSkills, names)
		})
	}
}

func TestDiscoverSkillByPath(t *testing.T) {
	tests := []struct {
		name      string
		skillPath string
		stubs     func(*httpmock.Registry)
		wantName  string
		wantNS    string
		wantErr   string
	}{
		{
			name:      "discovers skill by path",
			skillPath: "skills/code-review",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/contents/skills"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"name": "code-review", "path": "skills/code-review", "sha": "tree-sha", "type": "dir"},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree-sha", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "blob-sha"},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blob-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "blob-sha", "encoding": "base64", "content": "IyBTa2lsbA==",
					}))
			},
			wantName: "code-review",
		},
		{
			name:      "namespaced path sets namespace",
			skillPath: "skills/monalisa/issue-triage",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/contents/skills%2Fmonalisa"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"name": "issue-triage", "path": "skills/monalisa/issue-triage", "sha": "tree-sha", "type": "dir"},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree-sha", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "blob-sha"},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blob-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "blob-sha", "encoding": "base64", "content": "IyBTa2lsbA==",
					}))
			},
			wantName: "issue-triage",
			wantNS:   "monalisa",
		},
		{
			name:      "parent path with spaces is URL encoded",
			skillPath: "my skills/code-review",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/contents/my%20skills"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"name": "code-review", "path": "my skills/code-review", "sha": "tree-sha", "type": "dir"},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree-sha", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "blob-sha"},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blob-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "blob-sha", "encoding": "base64", "content": "IyBTa2lsbA==",
					}))
			},
			wantName: "code-review",
		},
		{
			name:      "strips trailing SKILL.md from path",
			skillPath: "skills/code-review/SKILL.md",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/contents/skills"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"name": "code-review", "path": "skills/code-review", "sha": "tree-sha", "type": "dir"},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree-sha", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "blob-sha"},
						},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blob-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "blob-sha", "encoding": "base64", "content": "IyBTa2lsbA==",
					}))
			},
			wantName: "code-review",
		},
		{
			name:      "invalid skill name",
			skillPath: "skills/.hidden-skill",
			wantErr:   "invalid skill name",
		},
		{
			name:      "skill directory not found",
			skillPath: "skills/nonexistent",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/contents/skills"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"name": "other-skill", "path": "skills/other-skill", "sha": "tree-sha", "type": "dir"},
					}))
			},
			wantErr: "skill directory",
		},
		{
			name:      "no SKILL.md in directory",
			skillPath: "skills/code-review",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/contents/skills"),
					httpmock.JSONResponse([]map[string]interface{}{
						{"name": "code-review", "path": "skills/code-review", "sha": "tree-sha", "type": "dir"},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree-sha"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree-sha", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "README.md", "type": "blob", "sha": "readme"},
						},
					}))
			},
			wantErr: "no SKILL.md found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.stubs != nil {
				tt.stubs(reg)
			}
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			skill, err := DiscoverSkillByPath(client, "github.com", "monalisa", "octocat-skills", "abc123", tt.skillPath)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, skill.Name)
			assert.Equal(t, tt.wantNS, skill.Namespace)
		})
	}
}

func TestDiscoverLocalSkills(t *testing.T) {
	tests := []struct {
		name       string
		createDir  bool
		setup      func(t *testing.T, dir string)
		wantSkills []string
		wantErr    string
	}{
		{
			name:      "discovers skills in skills/ directory",
			createDir: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				for _, name := range []string{"code-review", "issue-triage"} {
					skillDir := filepath.Join(dir, "skills", name)
					require.NoError(t, os.MkdirAll(skillDir, 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+name), 0o644))
				}
			},
			wantSkills: []string{"code-review", "issue-triage"},
		},
		{
			name:      "single skill at root",
			createDir: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(heredoc.Doc(`
				---
				name: root-skill
				---
				# Root
			`)), 0o644))
			},
			wantSkills: []string{"root-skill"},
		},
		{
			name:      "no skills found",
			createDir: true,
			setup: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Not a skill"), 0o644))
			},
			wantErr: "no skills found",
		},
		{
			name:    "nonexistent directory",
			setup:   func(t *testing.T, dir string) {},
			wantErr: "could not access",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "repo")
			if tt.createDir {
				require.NoError(t, os.MkdirAll(dir, 0o755))
			}
			tt.setup(t, dir)

			skills, err := DiscoverLocalSkills(dir)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var names []string
			for _, s := range skills {
				names = append(names, s.Name)
			}
			assert.ElementsMatch(t, tt.wantSkills, names)
		})
	}
}

func TestDiscoverLocalSkillsWithOptions(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, dir string)
		wantSkills []string
		wantErr    string
	}{
		{
			name: "returns hidden dir skills",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				skillDir := filepath.Join(dir, ".claude", "skills", "code-review")
				require.NoError(t, os.MkdirAll(skillDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# code-review"), 0o644))
			},
			wantSkills: []string{"code-review"},
		},
		{
			name: "mixed standard and hidden returns all",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				for _, p := range []string{"skills/standard", ".agents/skills/hidden"} {
					skillDir := filepath.Join(dir, filepath.FromSlash(p))
					require.NoError(t, os.MkdirAll(skillDir, 0o755))
					name := filepath.Base(p)
					require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# "+name), 0o644))
				}
			},
			wantSkills: []string{"standard", "hidden"},
		},
		{
			name:    "no skills at all",
			setup:   func(t *testing.T, _ string) { t.Helper() },
			wantErr: "no skills found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "repo")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			tt.setup(t, dir)

			skills, err := DiscoverLocalSkillsWithOptions(dir, DiscoverOptions{})
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var names []string
			for _, s := range skills {
				names = append(names, s.Name)
			}
			assert.ElementsMatch(t, tt.wantSkills, names)
		})
	}
}

func TestMatchesSkillPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantName string
	}{
		{name: "skills convention", path: "skills/code-review/SKILL.md", wantName: "code-review"},
		{name: "namespaced convention", path: "skills/monalisa/issue-triage/SKILL.md", wantName: "issue-triage"},
		{name: "plugins convention", path: "plugins/hubot/skills/pr-summary/SKILL.md", wantName: "pr-summary"},
		{name: "non-skill file", path: "README.md", wantName: ""},
		{name: "non-SKILL.md in skill dir", path: "skills/code-review/prompt.txt", wantName: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantName, MatchesSkillPath(tt.path))
		})
	}
}

func TestMatchSkillPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantName      string
		wantNamespace string
	}{
		{name: "skills convention", path: "skills/code-review/SKILL.md", wantName: "code-review", wantNamespace: ""},
		{name: "namespaced convention", path: "skills/monalisa/issue-triage/SKILL.md", wantName: "issue-triage", wantNamespace: "monalisa"},
		{name: "plugins convention", path: "plugins/hubot/skills/pr-summary/SKILL.md", wantName: "pr-summary", wantNamespace: "hubot"},
		{name: "non-skill file", path: "README.md", wantName: "", wantNamespace: ""},
		{name: "same name different namespace 1", path: "skills/kynan/commit/SKILL.md", wantName: "commit", wantNamespace: "kynan"},
		{name: "same name different namespace 2", path: "skills/will/commit/SKILL.md", wantName: "commit", wantNamespace: "will"},
		{name: "root convention", path: "my-skill/SKILL.md", wantName: "my-skill", wantNamespace: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, namespace := MatchSkillPath(tt.path)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantNamespace, namespace)
		})
	}
}

func TestDiscoverSkillFiles(t *testing.T) {
	tests := []struct {
		name      string
		stubs     func(*httpmock.Registry)
		wantPaths []string
		wantErr   string
	}{
		{
			name: "returns files with skill path prefix",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "sha1", "size": 10},
							{"path": "scripts/setup.sh", "type": "blob", "sha": "sha2", "size": 50},
							{"path": "scripts", "type": "tree", "sha": "treesub"},
						},
					}))
			},
			wantPaths: []string{"skills/code-review/SKILL.md", "skills/code-review/scripts/setup.sh"},
		},
		{
			name: "truncated tree falls back to walkTree",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123", "truncated": true, "tree": []map[string]interface{}{},
					}))
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123",
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "sha1", "size": 10},
						},
					}))
			},
			wantPaths: []string{"skills/code-review/SKILL.md"},
		},
		{
			name: "API error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: "could not fetch skill tree",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			files, err := DiscoverSkillFiles(client, "github.com", "monalisa", "octocat-skills", "tree123", "skills/code-review")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var paths []string
			for _, f := range files {
				paths = append(paths, f.Path)
			}
			assert.Equal(t, tt.wantPaths, paths)
		})
	}
}

func TestListSkillFiles(t *testing.T) {
	tests := []struct {
		name      string
		stubs     func(*httpmock.Registry)
		wantPaths []string
		wantErr   string
	}{
		{
			name: "returns relative paths",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123", "truncated": false,
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "sha1", "size": 10},
							{"path": "prompt.txt", "type": "blob", "sha": "sha2", "size": 20},
						},
					}))
			},
			wantPaths: []string{"SKILL.md", "prompt.txt"},
		},
		{
			name: "truncated tree falls back to walkTree with nested subtree",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123", "truncated": true, "tree": []map[string]interface{}{},
					}))
				// walkTree fetches the top-level tree non-recursively
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "tree123",
						"tree": []map[string]interface{}{
							{"path": "SKILL.md", "type": "blob", "sha": "sha1", "size": 10},
							{"path": "scripts", "type": "tree", "sha": "subtree1"},
						},
					}))
				// walkTree recurses into the "scripts" subtree
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/subtree1"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "subtree1",
						"tree": []map[string]interface{}{
							{"path": "setup.sh", "type": "blob", "sha": "sha2", "size": 50},
						},
					}))
			},
			wantPaths: []string{"SKILL.md", "scripts/setup.sh"},
		},
		{
			name: "API error",
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/trees/tree123"),
					httpmock.StatusStringResponse(500, "server error"))
			},
			wantErr: "could not fetch skill tree",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			files, err := ListSkillFiles(client, "github.com", "monalisa", "octocat-skills", "tree123")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			var paths []string
			for _, f := range files {
				paths = append(paths, f.Path)
			}
			assert.Equal(t, tt.wantPaths, paths)
		})
	}
}

func TestFetchDescriptionsConcurrent(t *testing.T) {
	tests := []struct {
		name      string
		skills    []Skill
		stubs     func(*httpmock.Registry)
		wantDescs []string
	}{
		{
			name: "fetches descriptions for skills without one",
			skills: []Skill{
				{Name: "code-review", BlobSHA: "blob1"},
				{Name: "issue-triage", Description: "already set"},
			},
			stubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/monalisa/octocat-skills/git/blobs/blob1"),
					httpmock.JSONResponse(map[string]interface{}{
						"sha": "blob1", "encoding": "base64",
						"content": "LS0tCm5hbWU6IGNvZGUtcmV2aWV3CmRlc2NyaXB0aW9uOiBSZXZpZXdzIFBScwotLS0KIyBUZXN0",
					}))
			},
			wantDescs: []string{"Reviews PRs", "already set"},
		},
		{
			name: "no-op when all descriptions set",
			skills: []Skill{
				{Name: "code-review", Description: "set"},
			},
			stubs:     func(reg *httpmock.Registry) {},
			wantDescs: []string{"set"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.stubs(reg)
			client := api.NewClientFromHTTP(&http.Client{Transport: reg})

			FetchDescriptionsConcurrent(client, "github.com", "monalisa", "octocat-skills", tt.skills, nil)
			var descs []string
			for _, s := range tt.skills {
				descs = append(descs, s.Description)
			}
			assert.Equal(t, tt.wantDescs, descs)
		})
	}
}
