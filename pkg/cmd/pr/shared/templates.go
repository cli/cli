package shared

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/githubtemplate"
	"github.com/cli/cli/v2/pkg/prompt"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
)

type issueTemplate struct {
	// I would have un-exported these fields, except `cli/shurcool-graphql` then cannot unmarshal them :/
	Gname string `graphql:"name"`
	Gbody string `graphql:"body"`
}

type pullRequestTemplate struct {
	// I would have un-exported these fields, except `cli/shurcool-graphql` then cannot unmarshal them :/
	Gname string `graphql:"filename"`
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

func (t *pullRequestTemplate) Name() string {
	return t.Gname
}

func (t *pullRequestTemplate) NameForSubmit() string {
	return ""
}

func (t *pullRequestTemplate) Body() []byte {
	return []byte(t.Gbody)
}

func listIssueTemplates(httpClient *http.Client, repo ghrepo.Interface) ([]Template, error) {
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

	ts := query.Repository.IssueTemplates
	templates := make([]Template, len(ts))
	for i := range templates {
		templates[i] = &ts[i]
	}

	return templates, nil
}

func listPullRequestTemplates(httpClient *http.Client, repo ghrepo.Interface) ([]Template, error) {
	var query struct {
		Repository struct {
			PullRequestTemplates []pullRequestTemplate
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	err := gql.QueryNamed(context.Background(), "PullRequestTemplates", &query, variables)
	if err != nil {
		return nil, err
	}

	ts := query.Repository.PullRequestTemplates
	templates := make([]Template, len(ts))
	for i := range templates {
		templates[i] = &ts[i]
	}

	return templates, nil
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
	detector   fd.Detector

	templates      []Template
	legacyTemplate Template

	didFetch   bool
	fetchError error
}

func NewTemplateManager(httpClient *http.Client, repo ghrepo.Interface, dir string, allowFS bool, isPR bool) *templateManager {
	cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
	return &templateManager{
		repo:       repo,
		rootDir:    dir,
		allowFS:    allowFS,
		isPR:       isPR,
		httpClient: httpClient,
		detector:   fd.NewDetector(cachedClient, repo.RepoHost()),
	}
}

func (m *templateManager) hasAPI() (bool, error) {
	if !m.isPR {
		return true, nil
	}

	features, err := m.detector.RepositoryFeatures()
	if err != nil {
		return false, err
	}

	return features.PullRequestTemplateQuery, nil
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
		lister := listIssueTemplates
		if m.isPR {
			lister = listPullRequestTemplates
		}
		templates, err := lister(m.httpClient, m.repo)
		if err != nil {
			return err
		}
		m.templates = templates
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
