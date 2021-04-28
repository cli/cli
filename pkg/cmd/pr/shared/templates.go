package shared

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/pkg/prompt"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

type issueTemplate struct {
	// I would have un-exported these fields, except `shurcool/graphql` then cannot unmarshal them :/
	Gname string `graphql:"name"`
	Gbody string `graphql:"body"`
}

func (t *issueTemplate) Name() string {
	return t.Gname
}

func (t *issueTemplate) NameForSubmit() string {
	return t.Gname
}

func (t *issueTemplate) Body() []byte {
	return []byte(t.Gbody)
}

func listIssueTemplates(httpClient *http.Client, repo ghrepo.Interface) ([]issueTemplate, error) {
	var query struct {
		Repository struct {
			IssueTemplates []issueTemplate
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	err := gql.QueryNamed(context.Background(), "IssueTemplates", &query, variables)
	if err != nil {
		return nil, err
	}

	return query.Repository.IssueTemplates, nil
}

func hasIssueTemplateSupport(httpClient *http.Client, hostname string) (bool, error) {
	if !ghinstance.IsEnterprise(hostname) {
		return true, nil
	}

	var featureDetection struct {
		Repository struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Repository: __type(name: \"Repository\")"`
		CreateIssueInput struct {
			InputFields []struct {
				Name string
			}
		} `graphql:"CreateIssueInput: __type(name: \"CreateIssueInput\")"`
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(hostname), httpClient)
	err := gql.QueryNamed(context.Background(), "IssueTemplates_fields", &featureDetection, nil)
	if err != nil {
		return false, err
	}

	var hasQuerySupport bool
	var hasMutationSupport bool
	for _, field := range featureDetection.Repository.Fields {
		if field.Name == "issueTemplates" {
			hasQuerySupport = true
		}
	}
	for _, field := range featureDetection.CreateIssueInput.InputFields {
		if field.Name == "issueTemplate" {
			hasMutationSupport = true
		}
	}

	return hasQuerySupport && hasMutationSupport, nil
}

type Template interface {
	Name() string
	NameForSubmit() string
	Body() []byte
}

type templateManager struct {
	repo       ghrepo.Interface
	rootDir    string
	allowFS    bool
	isPR       bool
	httpClient *http.Client

	cachedClient   *http.Client
	templates      []Template
	legacyTemplate Template

	didFetch   bool
	fetchError error
}

func NewTemplateManager(httpClient *http.Client, repo ghrepo.Interface, dir string, allowFS bool, isPR bool) *templateManager {
	return &templateManager{
		repo:       repo,
		rootDir:    dir,
		allowFS:    allowFS,
		isPR:       isPR,
		httpClient: httpClient,
	}
}

func (m *templateManager) hasAPI() (bool, error) {
	if m.isPR {
		return false, nil
	}
	if m.cachedClient == nil {
		m.cachedClient = api.NewCachedClient(m.httpClient, time.Hour*24)
	}
	return hasIssueTemplateSupport(m.cachedClient, m.repo.RepoHost())
}

func (m *templateManager) HasTemplates() (bool, error) {
	if err := m.memoizedFetch(); err != nil {
		return false, err
	}
	return len(m.templates) > 0, nil
}

func (m *templateManager) LegacyBody() []byte {
	if m.legacyTemplate == nil {
		return nil
	}
	return m.legacyTemplate.Body()
}

func (m *templateManager) Choose() (Template, error) {
	if err := m.memoizedFetch(); err != nil {
		return nil, err
	}
	if len(m.templates) == 0 {
		return nil, nil
	}

	names := make([]string, len(m.templates))
	for i, t := range m.templates {
		names[i] = t.Name()
	}

	blankOption := "Open a blank issue"
	if m.isPR {
		blankOption = "Open a blank pull request"
	}

	var selectedOption int
	err := prompt.SurveyAskOne(&survey.Select{
		Message: "Choose a template",
		Options: append(names, blankOption),
	}, &selectedOption)
	if err != nil {
		return nil, fmt.Errorf("could not prompt: %w", err)
	}

	if selectedOption == len(names) {
		return nil, nil
	}
	return m.templates[selectedOption], nil
}

func (m *templateManager) memoizedFetch() error {
	if m.didFetch {
		return m.fetchError
	}
	m.fetchError = m.fetch()
	m.didFetch = true
	return m.fetchError
}

func (m *templateManager) fetch() error {
	hasAPI, err := m.hasAPI()
	if err != nil {
		return err
	}

	if hasAPI {
		issueTemplates, err := listIssueTemplates(m.httpClient, m.repo)
		if err != nil {
			return err
		}
		m.templates = make([]Template, len(issueTemplates))
		for i := range issueTemplates {
			m.templates[i] = &issueTemplates[i]
		}
	}

	if !m.allowFS {
		return nil
	}

	dir := m.rootDir
	if dir == "" {
		var err error
		dir, err = git.ToplevelDir()
		if err != nil {
			return nil // abort silently
		}
	}

	filePattern := "ISSUE_TEMPLATE"
	if m.isPR {
		filePattern = "PULL_REQUEST_TEMPLATE"
	}

	if !hasAPI {
		issueTemplates := githubtemplate.FindNonLegacy(dir, filePattern)
		m.templates = make([]Template, len(issueTemplates))
		for i, t := range issueTemplates {
			m.templates[i] = &filesystemTemplate{path: t}
		}
	}

	if legacyTemplate := githubtemplate.FindLegacy(dir, filePattern); legacyTemplate != "" {
		m.legacyTemplate = &filesystemTemplate{path: legacyTemplate}
	}

	return nil
}

type filesystemTemplate struct {
	path string
}

func (t *filesystemTemplate) Name() string {
	return githubtemplate.ExtractName(t.path)
}

func (t *filesystemTemplate) NameForSubmit() string {
	return ""
}

func (t *filesystemTemplate) Body() []byte {
	return githubtemplate.ExtractContents(t.path)
}
