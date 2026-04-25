package search

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchRun_UnsupportedHost(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	cfg := config.NewBlankConfig()
	authCfg := cfg.Authentication()
	authCfg.SetDefaultHost("acme.ghes.com", "user")
	cfg.AuthenticationFunc = func() gh.AuthConfig {
		return authCfg
	}
	err := searchRun(&SearchOptions{
		IO:         ios,
		Query:      "terraform",
		Page:       1,
		Limit:      defaultLimit,
		HttpClient: func() (*http.Client, error) { return &http.Client{}, nil },
		Config:     func() (gh.Config, error) { return cfg, nil },
	})
	require.ErrorContains(t, err, "does not currently support GitHub Enterprise Server")
}

func TestNewCmdSearch(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts SearchOptions
		wantErr  string
	}{
		{
			name:     "query argument",
			args:     "terraform",
			wantOpts: SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
		},
		{
			name:     "with page flag",
			args:     "terraform --page 3",
			wantOpts: SearchOptions{Query: "terraform", Page: 3, Limit: defaultLimit},
		},
		{
			name:     "with limit flag",
			args:     "terraform --limit 5",
			wantOpts: SearchOptions{Query: "terraform", Page: 1, Limit: 5},
		},
		{
			name:     "with limit short flag",
			args:     "terraform -L 10",
			wantOpts: SearchOptions{Query: "terraform", Page: 1, Limit: 10},
		},
		{
			name:     "with owner flag",
			args:     "terraform --owner hashicorp",
			wantOpts: SearchOptions{Query: "terraform", Owner: "hashicorp", Page: 1, Limit: defaultLimit},
		},
		{
			name:    "no arguments",
			args:    "",
			wantErr: "cannot search: query argument required",
		},
		{
			name:    "invalid page",
			args:    "terraform --page 0",
			wantErr: "invalid page number: 0",
		},
		{
			name:    "query too short",
			args:    "a",
			wantErr: "search query must be at least 2 characters",
		},
		{
			name:    "query too short single char",
			args:    "x",
			wantErr: "search query must be at least 2 characters",
		},
		{
			name:    "invalid limit zero",
			args:    "terraform --limit 0",
			wantErr: "invalid limit: 0",
		},
		{
			name:    "invalid limit negative",
			args:    "terraform --limit -1",
			wantErr: "invalid limit: -1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}
			var gotOpts *SearchOptions
			cmd := NewCmdSearch(f, &telemetry.NoOpService{}, func(opts *SearchOptions) error {
				gotOpts = opts
				return nil
			})

			argv := []string{}
			if tt.args != "" {
				argv = strings.Fields(tt.args)
			}
			cmd.SetArgs(argv)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err := cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.Query, gotOpts.Query)
			assert.Equal(t, tt.wantOpts.Owner, gotOpts.Owner)
			assert.Equal(t, tt.wantOpts.Page, gotOpts.Page)
			assert.Equal(t, tt.wantOpts.Limit, gotOpts.Limit)
		})
	}
}

func TestSearchRun(t *testing.T) {
	const emptyCodeResponse = `{"total_count": 0, "incomplete_results": false, "items": []}`

	// stubKeywordSearch registers the HTTP stubs needed for a keyword search.
	// searchByKeyword fires up to 3 concurrent search/code requests (path,
	// owner, primary). Stubs are one-shot in httpmock, so we register one
	// per request.
	stubKeywordSearch := func(reg *httpmock.Registry, codeResponse string) {
		for range 3 {
			reg.Register(
				httpmock.REST("GET", "search/code"),
				httpmock.StringResponse(codeResponse),
			)
		}
	}

	tests := []struct {
		name       string
		opts       *SearchOptions
		tty        bool
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    string
	}{
		{
			name: "displays results in non-TTY",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 1, "incomplete_results": false, "items": [{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "repository": {"full_name": "github/awesome-skills"}}]}`)
			},
			wantStdout: "github/awesome-skills\tterraform\t\t0\n",
		},
		{
			name: "deduplicates results",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 3, "incomplete_results": false, "items": [{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "repository": {"full_name": "github/awesome-skills"}}, {"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "repository": {"full_name": "github/awesome-skills"}}, {"name": "SKILL.md", "path": "skills/terraform-aws/SKILL.md", "repository": {"full_name": "github/awesome-skills"}}]}`)
			},
			wantStdout: "github/awesome-skills\tterraform\t\t0\ngithub/awesome-skills\tterraform-aws\t\t0\n",
		},
		{
			name: "no results",
			tty:  true,
			opts: &SearchOptions{Query: "nonexistent", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, emptyCodeResponse)
			},
			wantErr: `no skills found matching "nonexistent"`,
		},
		{
			name: "nested skill path",
			tty:  false,
			opts: &SearchOptions{Query: "my-skill", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 1, "incomplete_results": false, "items": [{"name": "SKILL.md", "path": "skills/author/my-skill/SKILL.md", "repository": {"full_name": "org/repo"}}]}`)
			},
			wantStdout: "org/repo\tauthor/my-skill\t\t0\n",
		},
		{
			name: "ranks name-matching results first",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 3, "incomplete_results": false, "items": [
						{"name": "SKILL.md", "path": "skills/terraform-deploy/SKILL.md", "repository": {"full_name": "org/repo1"}},
						{"name": "SKILL.md", "path": "skills/terraform-plan/SKILL.md", "repository": {"full_name": "org/repo2"}},
						{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "repository": {"full_name": "org/repo3"}}
					]}`)
			},
			// exact name match "terraform" first, then partial matches alphabetically by score
			wantStdout: "org/repo3\tterraform\t\t0\norg/repo1\tterraform-deploy\t\t0\norg/repo2\tterraform-plan\t\t0\n",
		},
		{
			name: "caps total pages at 1000-result limit",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 5000, "incomplete_results": false, "items": [{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "repository": {"full_name": "org/repo"}}]}`)
			},
			// In non-TTY mode, no header or pagination text is shown
			wantStdout: "org/repo\tterraform\t\t0\n",
		},
		{
			name: "page beyond available results",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 999, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 1, "incomplete_results": false, "items": [{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "repository": {"full_name": "org/repo"}}]}`)
			},
			wantErr: `no skills found on page 999 for query "terraform"`,
		},
		{
			name: "namespaced skills are kept distinct in same repo",
			tty:  false,
			opts: &SearchOptions{Query: "commit", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 2, "incomplete_results": false, "items": [
					{"name": "SKILL.md", "path": "skills/kynan/commit/SKILL.md", "repository": {"full_name": "org/skills-repo"}},
					{"name": "SKILL.md", "path": "skills/will/commit/SKILL.md", "repository": {"full_name": "org/skills-repo"}}
				]}`)
			},
			wantStdout: "org/skills-repo\tkynan/commit\t\t0\norg/skills-repo\twill/commit\t\t0\n",
		},
		{
			name: "json output with selected fields",
			tty:  false,
			opts: func() *SearchOptions {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields([]string{"repo", "skillName", "stars"})
				return &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit, Exporter: exporter}
			}(),
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, `{"total_count": 1, "incomplete_results": false, "items": [{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "repository": {"full_name": "github/awesome-skills"}}]}`)
			},
			wantStdout: "[{\"repo\":\"github/awesome-skills\",\"skillName\":\"terraform\",\"stars\":0}]\n",
		},
		{
			name: "json output empty results",
			tty:  false,
			opts: func() *SearchOptions {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields([]string{"repo", "skillName"})
				return &SearchOptions{Query: "nonexistent", Page: 1, Limit: defaultLimit, Exporter: exporter}
			}(),
			httpStubs: func(reg *httpmock.Registry) {
				stubKeywordSearch(reg, emptyCodeResponse)
			},
			wantStdout: "[]\n",
		},
		{
			name: "rate limit error returns friendly message",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				// All search/code calls return 403 with x-ratelimit-remaining: 0
				for range 3 {
					reg.Register(
						httpmock.REST("GET", "search/code"),
						httpmock.WithHeader(
							httpmock.StatusJSONResponse(403, map[string]string{"message": "API rate limit exceeded"}),
							"x-ratelimit-remaining", "0",
						),
					)
				}
			},
			wantErr: rateLimitErrorMessage,
		},
		{
			name: "HTTP 429 returns rate limit error",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				for range 3 {
					reg.Register(
						httpmock.REST("GET", "search/code"),
						httpmock.StatusStringResponse(429, `{"message": "Too Many Requests"}`),
					)
				}
			},
			wantErr: rateLimitErrorMessage,
		},
		{
			name: "HTTP 403 with Retry-After returns rate limit error",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				for range 3 {
					reg.Register(
						httpmock.REST("GET", "search/code"),
						httpmock.WithHeader(
							httpmock.StatusJSONResponse(403, map[string]string{"message": "secondary rate limit"}),
							"Retry-After", "60",
						),
					)
				}
			},
			wantErr: rateLimitErrorMessage,
		},
		{
			name: "no results with owner scope",
			tty:  true,
			opts: &SearchOptions{Query: "nonexistent", Owner: "monalisa", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				// With --owner set, only path + primary searches fire (no owner search).
				for range 2 {
					reg.Register(
						httpmock.REST("GET", "search/code"),
						httpmock.StringResponse(emptyCodeResponse),
					)
				}
			},
			wantErr: `no skills found matching "nonexistent" from owner "monalisa"`,
		},
		{
			name: "enriches results with blob descriptions",
			tty:  false,
			opts: &SearchOptions{Query: "terraform", Page: 1, Limit: defaultLimit},
			httpStubs: func(reg *httpmock.Registry) {
				codeResponse := `{"total_count": 1, "incomplete_results": false, "items": [
					{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "sha": "abc123",
					 "repository": {"full_name": "org/repo"}}
				]}`
				stubKeywordSearch(reg, codeResponse)
				// Blob fetch for description enrichment
				reg.Register(
					httpmock.REST("GET", "repos/org/repo/git/blobs/abc123"),
					httpmock.JSONResponse(map[string]string{
						"content":  "LS0tCmRlc2NyaXB0aW9uOiBBdXRvbWF0ZXMgVGVycmFmb3JtIGluZnJhc3RydWN0dXJlCi0tLQojIFRlcnJhZm9ybSBTa2lsbAo=",
						"encoding": "base64",
					}),
				)
				// Repo stars fetch
				reg.Register(
					httpmock.REST("GET", "repos/org/repo"),
					httpmock.JSONResponse(map[string]int{"stargazers_count": 42}),
				)
			},
			wantStdout: "org/repo\tterraform\tAutomates Terraform infrastructure\t42\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			tt.opts.IO = ios
			tt.opts.Telemetry = &telemetry.NoOpService{}

			defer reg.Verify(t)
			err := searchRun(tt.opts)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func TestDeduplicateResults(t *testing.T) {
	items := []codeSearchItem{
		{Path: "skills/terraform/SKILL.md", Repository: codeSearchRepository{FullName: "org/repo"}},
		{Path: "skills/terraform/SKILL.md", Repository: codeSearchRepository{FullName: "org/repo"}},
		{Path: "skills/docker/SKILL.md", Repository: codeSearchRepository{FullName: "org/repo"}},
		{Path: "skills/terraform/SKILL.md", Repository: codeSearchRepository{FullName: "other/repo"}},
	}

	results := deduplicateResults(items)

	assert.Equal(t, 3, len(results))
	assert.Equal(t, "org/repo", results[0].Repo)
	assert.Equal(t, "org", results[0].Owner)
	assert.Equal(t, "repo", results[0].RepoName)
	assert.Equal(t, "terraform", results[0].SkillName)
	assert.Equal(t, "docker", results[1].SkillName)
	assert.Equal(t, "other/repo", results[2].Repo)
	assert.Equal(t, "other", results[2].Owner)
	assert.Equal(t, "terraform", results[2].SkillName)
}

func TestDeduplicateResults_Namespaced(t *testing.T) {
	items := []codeSearchItem{
		{Path: "skills/kynan/commit/SKILL.md", Repository: codeSearchRepository{FullName: "org/repo"}},
		{Path: "skills/will/commit/SKILL.md", Repository: codeSearchRepository{FullName: "org/repo"}},
		{Path: "skills/kynan/commit/SKILL.md", Repository: codeSearchRepository{FullName: "org/repo"}}, // duplicate
		{Path: "skills/commit/SKILL.md", Repository: codeSearchRepository{FullName: "org/repo"}},       // non-namespaced
	}

	results := deduplicateResults(items)

	require.Equal(t, 3, len(results))
	assert.Equal(t, "commit", results[0].SkillName)
	assert.Equal(t, "kynan", results[0].Namespace)
	assert.Equal(t, "commit", results[1].SkillName)
	assert.Equal(t, "will", results[1].Namespace)
	assert.Equal(t, "commit", results[2].SkillName)
	assert.Equal(t, "", results[2].Namespace)
}

func TestExtractSkillInfo(t *testing.T) {
	tests := []struct {
		path          string
		wantName      string
		wantNamespace string
	}{
		{"skills/terraform/SKILL.md", "terraform", ""},
		{"skills/author/my-skill/SKILL.md", "my-skill", "author"},
		{"SKILL.md", "", ""},
		{"skills/docker/SKILL.md", "docker", ""},
		// Root-level convention
		{"my-skill/SKILL.md", "my-skill", ""},
		// Plugins convention
		{"plugins/openai/skills/chat/SKILL.md", "chat", "openai"},
		// Non-matching paths should be filtered out
		{"random/nested/deep/SKILL.md", "", ""},
		{".hidden/SKILL.md", "", ""},
		// Same-name skills with different namespaces
		{"skills/kynan/commit/SKILL.md", "commit", "kynan"},
		{"skills/will/commit/SKILL.md", "commit", "will"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotName, gotNamespace := extractSkillInfo(tt.path)
			assert.Equal(t, tt.wantName, gotName)
			assert.Equal(t, tt.wantNamespace, gotNamespace)
		})
	}
}

func TestFilterByRelevance(t *testing.T) {
	skills := []skillResult{
		{Repo: "org/repo1", Owner: "org", RepoName: "repo1", SkillName: "terraform"},
		{Repo: "org/repo2", Owner: "org", RepoName: "repo2", SkillName: "docker"},
		{Repo: "terraform-corp/tools", Owner: "terraform-corp", RepoName: "tools", SkillName: "linter"},
		{Repo: "acme/terraform-tools", Owner: "acme", RepoName: "terraform-tools", SkillName: "validator"},
		{Repo: "x/y", Owner: "x", RepoName: "y", SkillName: "unrelated", Description: "terraform integration"},
		{Repo: "x/z", Owner: "x", RepoName: "z", SkillName: "noise"},
		{Repo: "org/repo3", Owner: "org", RepoName: "repo3", SkillName: "deploy", Namespace: "terraform"},
	}

	filtered := filterByRelevance(skills, "terraform")

	// Should keep: name match (terraform), owner match (terraform-corp),
	// repo name match (terraform-tools), description match (terraform integration),
	// namespace match (terraform/deploy).
	// Should drop: docker, noise.
	assert.Equal(t, 5, len(filtered))
	assert.Equal(t, "terraform", filtered[0].SkillName)
	assert.Equal(t, "linter", filtered[1].SkillName)
	assert.Equal(t, "validator", filtered[2].SkillName)
	assert.Equal(t, "unrelated", filtered[3].SkillName)
	assert.Equal(t, "deploy", filtered[4].SkillName)
	assert.Equal(t, "terraform", filtered[4].Namespace)
}

func TestRankByRelevance(t *testing.T) {
	skills := []skillResult{
		{Repo: "org/repo1", Owner: "org", SkillName: "devops"},
		{Repo: "org/repo2", Owner: "org", SkillName: "terraform-plan"},
		{Repo: "org/repo3", Owner: "org", SkillName: "docker", Description: "Manages terraform docker containers"},
		{Repo: "org/repo4", Owner: "org", SkillName: "terraform"},
	}

	rankByRelevance(skills, "terraform")

	// Exact name match scores highest (3 000), then partial name (1 000),
	// then description match (100), then body-only (0).
	assert.Equal(t, "terraform", skills[0].SkillName)
	assert.Equal(t, "terraform-plan", skills[1].SkillName)
	assert.Equal(t, "docker", skills[2].SkillName)
	assert.Equal(t, "devops", skills[3].SkillName)
}

func TestRankByRelevanceStarsTiebreak(t *testing.T) {
	skills := []skillResult{
		{Repo: "small/repo", Owner: "small", SkillName: "terraform", Stars: 10},
		{Repo: "big/repo", Owner: "big", SkillName: "terraform", Stars: 5000},
	}

	rankByRelevance(skills, "terraform")

	// Both have exact name match; big/repo wins on stars tiebreak
	assert.Equal(t, "big/repo", skills[0].Repo)
	assert.Equal(t, "small/repo", skills[1].Repo)
}

func TestFormatStars(t *testing.T) {
	assert.Equal(t, "0", formatStars(0))
	assert.Equal(t, "42", formatStars(42))
	assert.Equal(t, "999", formatStars(999))
	assert.Equal(t, "1.0k", formatStars(1000))
	assert.Equal(t, "1.7k", formatStars(1700))
	assert.Equal(t, "12.5k", formatStars(12500))
}

func TestQualifiedName(t *testing.T) {
	tests := []struct {
		name  string
		skill skillResult
		want  string
	}{
		{
			name:  "no namespace",
			skill: skillResult{SkillName: "terraform"},
			want:  "terraform",
		},
		{
			name:  "with namespace",
			skill: skillResult{SkillName: "commit", Namespace: "kynan"},
			want:  "kynan/commit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.skill.qualifiedName())
		})
	}
}

func TestDeduplicateByName_Namespaced(t *testing.T) {
	// Skills with the same base name but different namespaces should
	// be treated as distinct and not collapsed against each other.
	skills := []skillResult{
		{Repo: "org/repo1", SkillName: "commit", Namespace: "kynan"},
		{Repo: "org/repo2", SkillName: "commit", Namespace: "will"},
		{Repo: "org/repo3", SkillName: "commit"},
		{Repo: "org/repo4", SkillName: "commit", Namespace: "kynan"},
		{Repo: "org/repo5", SkillName: "commit", Namespace: "kynan"},
		{Repo: "org/repo6", SkillName: "commit", Namespace: "kynan"}, // should be capped (4th kynan/commit)
	}

	result := deduplicateByName(skills)

	// kynan/commit capped at 3, will/commit has 1, bare commit has 1 = 5 total
	require.Equal(t, 5, len(result))
	assert.Equal(t, "kynan", result[0].Namespace)
	assert.Equal(t, "will", result[1].Namespace)
	assert.Equal(t, "", result[2].Namespace)
	assert.Equal(t, "kynan", result[3].Namespace)
	assert.Equal(t, "kynan", result[4].Namespace)
	// repo6 should have been dropped
	for _, s := range result {
		assert.NotEqual(t, "org/repo6", s.Repo)
	}
}

// TestSearchRun_TelemetryRecordsInstallFromResults verifies that when a
// user searches, picks one or more results interactively, and proceeds to
// install them, the search command records a telemetry event capturing
// that the search led to an install attempt. This is the key signal for
// measuring the value of search results: of the searches that ran, how
// many converted to an install?
func TestSearchRun_TelemetryRecordsInstallFromResults(t *testing.T) {
	codeResponse := `{"total_count": 1, "incomplete_results": false, "items": [
		{"name": "SKILL.md", "path": "skills/terraform/SKILL.md", "sha": "abc123",
		 "repository": {"full_name": "org/repo"}}
	]}`

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	// Keyword search fires path + owner + primary (3 requests).
	for range 3 {
		reg.Register(
			httpmock.REST("GET", "search/code"),
			httpmock.StringResponse(codeResponse),
		)
	}

	ios, _, _, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)
	ios.SetStdinTTY(true)

	pm := &prompter.PrompterMock{
		MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
			// Select the single result.
			return []int{0}, nil
		},
		SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
			// First Select: target agent (0). Second Select: scope (0).
			return 0, nil
		},
	}

	recorder := &telemetry.EventRecorderSpy{}

	err := searchRun(&SearchOptions{
		IO:             ios,
		HttpClient:     func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		Config:         func() (gh.Config, error) { return config.NewBlankConfig(), nil },
		Prompter:       pm,
		Telemetry:      recorder,
		ExecutablePath: "/nonexistent/gh", // install subprocess will fail; failures are logged, not fatal.
		Query:          "terraform",
		Page:           1,
		Limit:          defaultLimit,
	})
	require.NoError(t, err)

	// The search command no longer records a separate skill_search event;
	// only the follow-up skill_search_install event fires when the user
	// proceeds to install from the results.
	require.Len(t, recorder.Events, 1)

	installEvent := recorder.Events[0]
	assert.Equal(t, "skill_search_install", installEvent.Type,
		"an install triggered from search results should be recorded as a distinct event")
	assert.Equal(t, int64(1), installEvent.Measures["install_count"],
		"install_count captures how many results the user chose to install")
	// The skill_search_install event must not carry the query or owner:
	// these were intentionally removed so that installs from search are
	// not linked back to the search terms at the telemetry layer.
	assert.Empty(t, installEvent.Dimensions["query"],
		"skill_search_install must not record the search query")
	assert.Empty(t, installEvent.Dimensions["owner"],
		"skill_search_install must not record the search owner filter")
}
