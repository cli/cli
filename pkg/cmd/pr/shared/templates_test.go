package shared

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/prompt"
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
				{"name": "Bug report", "body": "I found a problem"},
				{"name": "Feature request", "body": "I need a feature"}
			]
		}}}`))

	m := templateManager{
		repo:       ghrepo.NewWithHost("OWNER", "REPO", "example.com"),
		rootDir:    rootDir,
		allowFS:    true,
		isPR:       false,
		httpClient: httpClient,
		detector:   &fd.EnabledDetectorMock{},
	}

	hasTemplates, err := m.HasTemplates()
	assert.NoError(t, err)
	assert.True(t, hasTemplates)

	assert.Equal(t, "LEGACY", string(m.LegacyBody()))

	as := prompt.NewAskStubber(t)
	as.StubPrompt("Choose a template").
		AssertOptions([]string{"Bug report", "Feature request", "Open a blank issue"}).
		AnswerWith("Feature request")

	tpl, err := m.Choose()

	assert.NoError(t, err)
	assert.Equal(t, "Feature request", tpl.NameForSubmit())
	assert.Equal(t, "I need a feature", string(tpl.Body()))
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

	m := templateManager{
		repo:       ghrepo.NewWithHost("OWNER", "REPO", "example.com"),
		rootDir:    rootDir,
		allowFS:    true,
		isPR:       true,
		httpClient: httpClient,
		detector:   &fd.EnabledDetectorMock{},
	}

	hasTemplates, err := m.HasTemplates()
	assert.NoError(t, err)
	assert.True(t, hasTemplates)

	assert.Equal(t, "LEGACY", string(m.LegacyBody()))

	as := prompt.NewAskStubber(t)
	as.StubPrompt("Choose a template").
		AssertOptions([]string{"bug_pr.md", "feature_pr.md", "Open a blank pull request"}).
		AnswerWith("bug_pr.md")

	tpl, err := m.Choose()

	assert.NoError(t, err)
	assert.Equal(t, "", tpl.NameForSubmit())
	assert.Equal(t, "I fixed a problem", string(tpl.Body()))
}
