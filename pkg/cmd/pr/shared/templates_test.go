package shared

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestTemplateManager_hasAPI(t *testing.T) {
	rootDir := t.TempDir()
	legacyTemplateFile := filepath.Join(rootDir, ".github", "ISSUE_TEMPLATE.md")
	_ = os.MkdirAll(filepath.Dir(legacyTemplateFile), 0755)
	_ = os.WriteFile(legacyTemplateFile, []byte("LEGACY"), 0644)

	tr := httpmock.Registry{}
	httpClient := &http.Client{Transport: &tr}
	defer tr.Verify(t)

	tr.Register(
		httpmock.GraphQL(`query IssueTemplates\b`),
		httpmock.StringResponse(`{"data":{"repository":{
			"issueTemplates": [
				{"name": "Bug report", "body": "I found a problem", "title": "bug: "},
				{"name": "Feature request", "body": "I need a feature", "title": "request: "}
			]
		}}}`))

	pm := &prompter.PrompterMock{}
	pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
		if p == "Choose a template" {
			return prompter.IndexFor(opts, "Feature request")
		} else {
			return -1, prompter.NoSuchPromptErr(p)
		}
	}

	m := templateManager{
		repo:       ghrepo.NewWithHost("OWNER", "REPO", "example.com"),
		rootDir:    rootDir,
		allowFS:    true,
		isPR:       false,
		httpClient: httpClient,
		detector:   &fd.EnabledDetectorMock{},
		prompter:   pm,
	}

	hasTemplates, err := m.HasTemplates()
	assert.NoError(t, err)
	assert.True(t, hasTemplates)

	assert.Equal(t, "LEGACY", string(m.LegacyBody()))

	tpl, err := m.Choose()

	assert.NoError(t, err)
	assert.Equal(t, "Feature request", tpl.NameForSubmit())
	assert.Equal(t, "I need a feature", string(tpl.Body()))
	assert.Equal(t, "request: ", tpl.Title())
}

func TestTemplateManager_hasAPI_PullRequest(t *testing.T) {
	rootDir := t.TempDir()
	legacyTemplateFile := filepath.Join(rootDir, ".github", "PULL_REQUEST_TEMPLATE.md")
	_ = os.MkdirAll(filepath.Dir(legacyTemplateFile), 0755)
	_ = os.WriteFile(legacyTemplateFile, []byte("LEGACY"), 0644)

	tr := httpmock.Registry{}
	httpClient := &http.Client{Transport: &tr}
	defer tr.Verify(t)

	tr.Register(
		httpmock.GraphQL(`query PullRequestTemplates\b`),
		httpmock.StringResponse(`{"data":{"repository":{
			"pullRequestTemplates": [
				{"filename": "bug_pr.md", "body": "I fixed a problem"},
				{"filename": "feature_pr.md", "body": "I added a feature"}
			]
		}}}`))

	pm := &prompter.PrompterMock{}
	pm.SelectFunc = func(p, _ string, opts []string) (int, error) {
		if p == "Choose a template" {
			return prompter.IndexFor(opts, "bug_pr.md")
		} else {
			return -1, prompter.NoSuchPromptErr(p)
		}
	}
	m := templateManager{
		repo:       ghrepo.NewWithHost("OWNER", "REPO", "example.com"),
		rootDir:    rootDir,
		allowFS:    true,
		isPR:       true,
		httpClient: httpClient,
		detector:   &fd.EnabledDetectorMock{},
		prompter:   pm,
	}

	hasTemplates, err := m.HasTemplates()
	assert.NoError(t, err)
	assert.True(t, hasTemplates)

	assert.Equal(t, "LEGACY", string(m.LegacyBody()))

	tpl, err := m.Choose()

	assert.NoError(t, err)
	assert.Equal(t, "", tpl.NameForSubmit())
	assert.Equal(t, "I fixed a problem", string(tpl.Body()))
	assert.Equal(t, "", tpl.Title())
}

func TestTemplateManagerSelect(t *testing.T) {
	tests := []struct {
		name         string
		isPR         bool
		templateName string
		wantTemplate Template
		wantErr      bool
		errMsg       string
		httpStubs    func(*httpmock.Registry)
	}{
		{
			name:         "no templates found",
			templateName: "Bug report",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueTemplates\b`),
					httpmock.StringResponse(`{"data":{"repository":{"issueTemplates":[]}}}`),
				)
			},
			wantErr: true,
			errMsg:  "no templates found",
		},
		{
			name:         "no matching templates found",
			templateName: "Unknown report",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueTemplates\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issueTemplates": [
						{ "name": "Bug report", "body": "I found a problem" },
						{ "name": "Feature request", "body": "I need a feature" }
					] } } }`),
				)
			},
			wantErr: true,
			errMsg:  `template "Unknown report" not found`,
		},
		{
			name:         "matching issue template found",
			templateName: "Bug report",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueTemplates\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "issueTemplates": [
						{ "name": "Bug report", "body": "I found a problem" },
						{ "name": "Feature request", "body": "I need a feature" }
					] } } }`),
				)
			},
			wantTemplate: &issueTemplate{
				Gname: "Bug report",
				Gbody: "I found a problem",
			},
		},
		{
			name:         "matching pull request template found",
			isPR:         true,
			templateName: "feature.md",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestTemplates\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "PullRequestTemplates": [
						{ "filename": "bug.md", "body": "I fixed a problem" },
						{ "filename": "feature.md", "body": "I made a feature" }
					] } } }`),
				)
			},
			wantTemplate: &pullRequestTemplate{
				Gname: "feature.md",
				Gbody: "I made a feature",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			m := templateManager{
				repo:       ghrepo.NewWithHost("OWNER", "REPO", "example.com"),
				allowFS:    false,
				isPR:       tt.isPR,
				httpClient: &http.Client{Transport: reg},
				detector:   &fd.EnabledDetectorMock{},
			}

			tmpl, err := m.Select(tt.templateName)

			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantTemplate != nil {
				assert.Equal(t, tt.wantTemplate.Name(), tmpl.Name())
				assert.Equal(t, tt.wantTemplate.Body(), tmpl.Body())
			}
		})
	}
}

func TestTemplateManagerSelect_IssueTemplateByFilenameFallback(t *testing.T) {
	rootDir := t.TempDir()
	templateFile := filepath.Join(rootDir, ".github", "ISSUE_TEMPLATE", "task.md")
	_ = os.MkdirAll(filepath.Dir(templateFile), 0755)
	_ = os.WriteFile(templateFile, []byte(`---
name: Task
title: 'task: '
---

Describe the task
`), 0644)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query IssueTemplates\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "issueTemplates": [
			{ "name": "Task", "body": "Describe the task", "title": "task: " }
		] } } }`),
	)

	m := templateManager{
		repo:       ghrepo.NewWithHost("OWNER", "REPO", "example.com"),
		rootDir:    rootDir,
		allowFS:    true,
		isPR:       false,
		httpClient: &http.Client{Transport: reg},
		detector:   &fd.EnabledDetectorMock{},
	}

	tmpl, err := m.Select("task.md")

	assert.NoError(t, err)
	assert.Equal(t, "Task", tmpl.Name())
	assert.Equal(t, "Task", tmpl.NameForSubmit())
	assert.Equal(t, "task: ", tmpl.Title())
	assert.Equal(t, "Describe the task\n", string(tmpl.Body()))
}
