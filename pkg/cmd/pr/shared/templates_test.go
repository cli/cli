package shared

import (
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/stretchr/testify/assert"
)

func TestTemplateManager_hasAPI(t *testing.T) {
	rootDir := t.TempDir()
	legacyTemplateFile := filepath.Join(rootDir, ".github", "ISSUE_TEMPLATE.md")
	_ = os.MkdirAll(filepath.Dir(legacyTemplateFile), 0755)
	_ = ioutil.WriteFile(legacyTemplateFile, []byte("LEGACY"), 0644)

	tr := httpmock.Registry{}
	httpClient := &http.Client{Transport: &tr}
	defer tr.Verify(t)

	tr.Register(
		httpmock.GraphQL(`query IssueTemplates_fields\b`),
		httpmock.StringResponse(`{"data":{
			"Repository": {
				"fields": [
					{"name": "foo"},
					{"name": "issueTemplates"}
				]
			},
			"CreateIssueInput": {
				"inputFields": [
					{"name": "foo"},
					{"name": "issueTemplate"}
				]
			}
		}}`))
	tr.Register(
		httpmock.GraphQL(`query IssueTemplates\b`),
		httpmock.StringResponse(`{"data":{"repository":{
			"issueTemplates": [
				{"name": "Bug report", "body": "I found a problem"},
				{"name": "Feature request", "body": "I need a feature"}
			]
		}}}`))

	m := templateManager{
		repo:         ghrepo.NewWithHost("OWNER", "REPO", "example.com"),
		rootDir:      rootDir,
		allowFS:      true,
		isPR:         false,
		httpClient:   httpClient,
		cachedClient: httpClient,
	}

	hasTemplates, err := m.HasTemplates()
	assert.NoError(t, err)
	assert.True(t, hasTemplates)

	assert.Equal(t, "LEGACY", string(m.LegacyBody()))

	as, askRestore := prompt.InitAskStubber()
	defer askRestore()

	as.StubOne(1) // choose "Feature Request"
	tpl, err := m.Choose()
	assert.NoError(t, err)
	assert.Equal(t, "Feature request", tpl.NameForSubmit())
	assert.Equal(t, "I need a feature", string(tpl.Body()))
}
