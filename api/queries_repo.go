package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// Repository contains information about a GitHub repo
type Repository struct {
	ID                       string
	Name                     string
	NameWithOwner            string
	Owner                    RepositoryOwner
	Parent                   *Repository
	TemplateRepository       *Repository
	Description              string
	HomepageURL              string
	OpenGraphImageURL        string
	UsesCustomOpenGraphImage bool
	URL                      string
	SSHURL                   string
	MirrorURL                string
	SecurityPolicyURL        string

	CreatedAt time.Time
	PushedAt  *time.Time
	UpdatedAt time.Time

	IsBlankIssuesEnabled    bool
	IsSecurityPolicyEnabled bool
	HasIssuesEnabled        bool
	HasProjectsEnabled      bool
	HasWikiEnabled          bool
	MergeCommitAllowed      bool
	SquashMergeAllowed      bool
	RebaseMergeAllowed      bool

	ForkCount      int
	StargazerCount int
	Watchers       struct {
		TotalCount int `json:"totalCount"`
	}
	Issues struct {
		TotalCount int `json:"totalCount"`
	}
	PullRequests struct {
		TotalCount int `json:"totalCount"`
	}

	CodeOfConduct                 *CodeOfConduct
	ContactLinks                  []ContactLink
	DefaultBranchRef              BranchRef
	DeleteBranchOnMerge           bool
	DiskUsage                     int
	FundingLinks                  []FundingLink
	IsArchived                    bool
	IsEmpty                       bool
	IsFork                        bool
	IsInOrganization              bool
	IsMirror                      bool
	IsPrivate                     bool
	IsTemplate                    bool
	IsUserConfigurationRepository bool
	LicenseInfo                   *RepositoryLicense
	ViewerCanAdminister           bool
	ViewerDefaultCommitEmail      string
	ViewerDefaultMergeMethod      string
	ViewerHasStarred              bool
	ViewerPermission              string
	ViewerPossibleCommitEmails    []string
	ViewerSubscription            string

	RepositoryTopics struct {
		Nodes []struct {
			Topic RepositoryTopic
		}
	}
	PrimaryLanguage *CodingLanguage
	Languages       struct {
		Edges []struct {
			Size int            `json:"size"`
			Node CodingLanguage `json:"node"`
		}
	}
	IssueTemplates       []IssueTemplate
	PullRequestTemplates []PullRequestTemplate
	Labels               struct {
		Nodes []IssueLabel
	}
	Milestones struct {
		Nodes []Milestone
	}
	LatestRelease *RepositoryRelease

	AssignableUsers struct {
		Nodes []GitHubUser
	}
	MentionableUsers struct {
		Nodes []GitHubUser
	}
	Projects struct {
		Nodes []RepoProject
	}

	// pseudo-field that keeps track of host name of this repo
	hostname string
}

// RepositoryOwner is the owner of a GitHub repository
type RepositoryOwner struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

type GitHubUser struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

// BranchRef is the branch name in a GitHub repository
type BranchRef struct {
	Name string `json:"name"`
}

type CodeOfConduct struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type RepositoryLicense struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
}

type ContactLink struct {
	About string `json:"about"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type FundingLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

type CodingLanguage struct {
	Name string `json:"name"`
}

type IssueTemplate struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Body  string `json:"body"`
	About string `json:"about"`
}

type PullRequestTemplate struct {
	Filename string `json:"filename"`
	Body     string `json:"body"`
}

type RepositoryTopic struct {
	Name string `json:"name"`
}

type RepositoryRelease struct {
	Name        string    `json:"name"`
	TagName     string    `json:"tagName"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"publishedAt"`
}

type IssueLabel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

type License struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// RepoOwner is the login name of the owner
func (r Repository) RepoOwner() string {
	return r.Owner.Login
}

// RepoName is the name of the repository
func (r Repository) RepoName() string {
	return r.Name
}

// RepoHost is the GitHub hostname of the repository
func (r Repository) RepoHost() string {
	return r.hostname
}

// ViewerCanPush is true when the requesting user has push access
func (r Repository) ViewerCanPush() bool {
	switch r.ViewerPermission {
	case "ADMIN", "MAINTAIN", "WRITE":
		return true
	default:
		return false
	}
}

// ViewerCanTriage is true when the requesting user can triage issues and pull requests
func (r Repository) ViewerCanTriage() bool {
	switch r.ViewerPermission {
	case "ADMIN", "MAINTAIN", "WRITE", "TRIAGE":
		return true
	default:
		return false
	}
}

func FetchRepository(client *Client, repo ghrepo.Interface, fields []string) (*Repository, error) {
	query := fmt.Sprintf(`query RepositoryInfo($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {%s}
	}`, RepositoryGraphQL(fields))

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}

	var result struct {
		Repository Repository
	}
	if err := client.GraphQL(repo.RepoHost(), query, variables, &result); err != nil {
		return nil, err
	}
	return InitRepoHostname(&result.Repository, repo.RepoHost()), nil
}

func GitHubRepo(client *Client, repo ghrepo.Interface) (*Repository, error) {
	query := `
	fragment repo on Repository {
		id
		name
		owner { login }
		hasIssuesEnabled
		description
		hasWikiEnabled
		viewerPermission
		defaultBranchRef {
			name
		}
	}

	query RepositoryInfo($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {
			...repo
			parent {
				...repo
			}
			mergeCommitAllowed
			rebaseMergeAllowed
			squashMergeAllowed
		}
	}`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}

	result := struct {
		Repository Repository
	}{}
	err := client.GraphQL(repo.RepoHost(), query, variables, &result)

	if err != nil {
		return nil, err
	}

	return InitRepoHostname(&result.Repository, repo.RepoHost()), nil
}

func RepoDefaultBranch(client *Client, repo ghrepo.Interface) (string, error) {
	if r, ok := repo.(*Repository); ok && r.DefaultBranchRef.Name != "" {
		return r.DefaultBranchRef.Name, nil
	}

	r, err := GitHubRepo(client, repo)
	if err != nil {
		return "", err
	}
	return r.DefaultBranchRef.Name, nil
}

func CanPushToRepo(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	if r, ok := repo.(*Repository); ok && r.ViewerPermission != "" {
		return r.ViewerCanPush(), nil
	}

	apiClient := NewClientFromHTTP(httpClient)
	r, err := GitHubRepo(apiClient, repo)
	if err != nil {
		return false, err
	}
	return r.ViewerCanPush(), nil
}

// RepoParent finds out the parent repository of a fork
func RepoParent(client *Client, repo ghrepo.Interface) (ghrepo.Interface, error) {
	var query struct {
		Repository struct {
			Parent *struct {
				Name  string
				Owner struct {
					Login string
				}
			}
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	gql := graphQLClient(client.http, repo.RepoHost())
	err := gql.QueryNamed(context.Background(), "RepositoryFindParent", &query, variables)
	if err != nil {
		return nil, err
	}
	if query.Repository.Parent == nil {
		return nil, nil
	}

	parent := ghrepo.NewWithHost(query.Repository.Parent.Owner.Login, query.Repository.Parent.Name, repo.RepoHost())
	return parent, nil
}

// RepoNetworkResult describes the relationship between related repositories
type RepoNetworkResult struct {
	ViewerLogin  string
	Repositories []*Repository
}

// RepoNetwork inspects the relationship between multiple GitHub repositories
func RepoNetwork(client *Client, repos []ghrepo.Interface) (RepoNetworkResult, error) {
	var hostname string
	if len(repos) > 0 {
		hostname = repos[0].RepoHost()
	}

	queries := make([]string, 0, len(repos))
	for i, repo := range repos {
		queries = append(queries, fmt.Sprintf(`
		repo_%03d: repository(owner: %q, name: %q) {
			...repo
			parent {
				...repo
			}
		}
		`, i, repo.RepoOwner(), repo.RepoName()))
	}

	// Since the query is constructed dynamically, we can't parse a response
	// format using a static struct. Instead, hold the raw JSON data until we
	// decide how to parse it manually.
	graphqlResult := make(map[string]*json.RawMessage)
	var result RepoNetworkResult

	err := client.GraphQL(hostname, fmt.Sprintf(`
	fragment repo on Repository {
		id
		name
		owner { login }
		viewerPermission
		defaultBranchRef {
			name
		}
		isPrivate
	}
	query RepositoryNetwork {
		viewer { login }
		%s
	}
	`, strings.Join(queries, "")), nil, &graphqlResult)
	graphqlError, isGraphQLError := err.(*GraphQLErrorResponse)
	if isGraphQLError {
		// If the only errors are that certain repositories are not found,
		// continue processing this response instead of returning an error
		tolerated := true
		for _, ge := range graphqlError.Errors {
			if ge.Type != "NOT_FOUND" {
				tolerated = false
			}
		}
		if tolerated {
			err = nil
		}
	}
	if err != nil {
		return result, err
	}

	keys := make([]string, 0, len(graphqlResult))
	for key := range graphqlResult {
		keys = append(keys, key)
	}
	// sort keys to ensure `repo_{N}` entries are processed in order
	sort.Strings(keys)

	// Iterate over keys of GraphQL response data and, based on its name,
	// dynamically allocate the target struct an individual message gets decoded to.
	for _, name := range keys {
		jsonMessage := graphqlResult[name]
		if name == "viewer" {
			viewerResult := struct {
				Login string
			}{}
			decoder := json.NewDecoder(bytes.NewReader([]byte(*jsonMessage)))
			if err := decoder.Decode(&viewerResult); err != nil {
				return result, err
			}
			result.ViewerLogin = viewerResult.Login
		} else if strings.HasPrefix(name, "repo_") {
			if jsonMessage == nil {
				result.Repositories = append(result.Repositories, nil)
				continue
			}
			var repo Repository
			decoder := json.NewDecoder(bytes.NewReader(*jsonMessage))
			if err := decoder.Decode(&repo); err != nil {
				return result, err
			}
			result.Repositories = append(result.Repositories, InitRepoHostname(&repo, hostname))
		} else {
			return result, fmt.Errorf("unknown GraphQL result key %q", name)
		}
	}
	return result, nil
}

func InitRepoHostname(repo *Repository, hostname string) *Repository {
	repo.hostname = hostname
	if repo.Parent != nil {
		repo.Parent.hostname = hostname
	}
	return repo
}

// RepositoryV3 is the repository result from GitHub API v3
type repositoryV3 struct {
	NodeID    string `json:"node_id"`
	Name      string
	CreatedAt time.Time `json:"created_at"`
	Owner     struct {
		Login string
	}
	Private bool
	HTMLUrl string `json:"html_url"`
	Parent  *repositoryV3
}

// ForkRepo forks the repository on GitHub and returns the new repository
func ForkRepo(client *Client, repo ghrepo.Interface, org string) (*Repository, error) {
	path := fmt.Sprintf("repos/%s/forks", ghrepo.FullName(repo))

	params := map[string]interface{}{}
	if org != "" {
		params["organization"] = org
	}

	body := &bytes.Buffer{}
	enc := json.NewEncoder(body)
	if err := enc.Encode(params); err != nil {
		return nil, err
	}

	result := repositoryV3{}
	err := client.REST(repo.RepoHost(), "POST", path, body, &result)
	if err != nil {
		return nil, err
	}

	return &Repository{
		ID:        result.NodeID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt,
		Owner: RepositoryOwner{
			Login: result.Owner.Login,
		},
		ViewerPermission: "WRITE",
		hostname:         repo.RepoHost(),
	}, nil
}

// RepoFindForks finds forks of the repo that are affiliated with the viewer
func RepoFindForks(client *Client, repo ghrepo.Interface, limit int) ([]*Repository, error) {
	result := struct {
		Repository struct {
			Forks struct {
				Nodes []Repository
			}
		}
	}{}

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
		"limit": limit,
	}

	if err := client.GraphQL(repo.RepoHost(), `
	query RepositoryFindFork($owner: String!, $repo: String!, $limit: Int!) {
		repository(owner: $owner, name: $repo) {
			forks(first: $limit, affiliations: [OWNER, COLLABORATOR]) {
				nodes {
					id
					name
					owner { login }
					url
					viewerPermission
				}
			}
		}
	}
	`, variables, &result); err != nil {
		return nil, err
	}

	var results []*Repository
	for _, r := range result.Repository.Forks.Nodes {
		// we check ViewerCanPush, even though we expect it to always be true per
		// `affiliations` condition, to guard against versions of GitHub with a
		// faulty `affiliations` implementation
		if !r.ViewerCanPush() {
			continue
		}
		results = append(results, InitRepoHostname(&r, repo.RepoHost()))
	}

	return results, nil
}

type RepoMetadataResult struct {
	AssignableUsers []RepoAssignee
	Labels          []RepoLabel
	Projects        []RepoProject
	Milestones      []RepoMilestone
	Teams           []OrgTeam
}

func (m *RepoMetadataResult) MembersToIDs(names []string) ([]string, error) {
	var ids []string
	for _, assigneeLogin := range names {
		found := false
		for _, u := range m.AssignableUsers {
			if strings.EqualFold(assigneeLogin, u.Login) {
				ids = append(ids, u.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", assigneeLogin)
		}
	}
	return ids, nil
}

func (m *RepoMetadataResult) TeamsToIDs(names []string) ([]string, error) {
	var ids []string
	for _, teamSlug := range names {
		found := false
		slug := teamSlug[strings.IndexRune(teamSlug, '/')+1:]
		for _, t := range m.Teams {
			if strings.EqualFold(slug, t.Slug) {
				ids = append(ids, t.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", teamSlug)
		}
	}
	return ids, nil
}

func (m *RepoMetadataResult) LabelsToIDs(names []string) ([]string, error) {
	var ids []string
	for _, labelName := range names {
		found := false
		for _, l := range m.Labels {
			if strings.EqualFold(labelName, l.Name) {
				ids = append(ids, l.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", labelName)
		}
	}
	return ids, nil
}

func (m *RepoMetadataResult) ProjectsToIDs(names []string) ([]string, error) {
	var ids []string
	for _, projectName := range names {
		found := false
		for _, p := range m.Projects {
			if strings.EqualFold(projectName, p.Name) {
				ids = append(ids, p.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", projectName)
		}
	}
	return ids, nil
}

func ProjectsToPaths(projects []RepoProject, names []string) ([]string, error) {
	var paths []string
	for _, projectName := range names {
		found := false
		for _, p := range projects {
			if strings.EqualFold(projectName, p.Name) {
				// format of ResourcePath: /OWNER/REPO/projects/PROJECT_NUMBER or /orgs/ORG/projects/PROJECT_NUMBER
				// required format of path: OWNER/REPO/PROJECT_NUMBER or ORG/PROJECT_NUMBER
				var path string
				pathParts := strings.Split(p.ResourcePath, "/")
				if pathParts[1] == "orgs" {
					path = fmt.Sprintf("%s/%s", pathParts[2], pathParts[4])
				} else {
					path = fmt.Sprintf("%s/%s/%s", pathParts[1], pathParts[2], pathParts[4])
				}
				paths = append(paths, path)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("'%s' not found", projectName)
		}
	}
	return paths, nil
}

func (m *RepoMetadataResult) MilestoneToID(title string) (string, error) {
	for _, m := range m.Milestones {
		if strings.EqualFold(title, m.Title) {
			return m.ID, nil
		}
	}
	return "", fmt.Errorf("'%s' not found", title)
}

func (m *RepoMetadataResult) Merge(m2 *RepoMetadataResult) {
	if len(m2.AssignableUsers) > 0 || len(m.AssignableUsers) == 0 {
		m.AssignableUsers = m2.AssignableUsers
	}

	if len(m2.Teams) > 0 || len(m.Teams) == 0 {
		m.Teams = m2.Teams
	}

	if len(m2.Labels) > 0 || len(m.Labels) == 0 {
		m.Labels = m2.Labels
	}

	if len(m2.Projects) > 0 || len(m.Projects) == 0 {
		m.Projects = m2.Projects
	}

	if len(m2.Milestones) > 0 || len(m.Milestones) == 0 {
		m.Milestones = m2.Milestones
	}
}

type RepoMetadataInput struct {
	Assignees  bool
	Reviewers  bool
	Labels     bool
	Projects   bool
	Milestones bool
}

// RepoMetadata pre-fetches the metadata for attaching to issues and pull requests
func RepoMetadata(client *Client, repo ghrepo.Interface, input RepoMetadataInput) (*RepoMetadataResult, error) {
	result := RepoMetadataResult{}
	errc := make(chan error)
	count := 0

	if input.Assignees || input.Reviewers {
		count++
		go func() {
			users, err := RepoAssignableUsers(client, repo)
			if err != nil {
				err = fmt.Errorf("error fetching assignees: %w", err)
			}
			result.AssignableUsers = users
			errc <- err
		}()
	}
	if input.Reviewers {
		count++
		go func() {
			teams, err := OrganizationTeams(client, repo)
			// TODO: better detection of non-org repos
			if err != nil && !strings.HasPrefix(err.Error(), "Could not resolve to an Organization") {
				errc <- fmt.Errorf("error fetching organization teams: %w", err)
				return
			}
			result.Teams = teams
			errc <- nil
		}()
	}
	if input.Labels {
		count++
		go func() {
			labels, err := RepoLabels(client, repo)
			if err != nil {
				err = fmt.Errorf("error fetching labels: %w", err)
			}
			result.Labels = labels
			errc <- err
		}()
	}
	if input.Projects {
		count++
		go func() {
			projects, err := RepoAndOrgProjects(client, repo)
			if err != nil {
				errc <- err
				return
			}
			result.Projects = projects
			errc <- nil
		}()
	}
	if input.Milestones {
		count++
		go func() {
			milestones, err := RepoMilestones(client, repo, "open")
			if err != nil {
				err = fmt.Errorf("error fetching milestones: %w", err)
			}
			result.Milestones = milestones
			errc <- err
		}()
	}

	var err error
	for i := 0; i < count; i++ {
		if e := <-errc; e != nil {
			err = e
		}
	}

	return &result, err
}

type RepoResolveInput struct {
	Assignees  []string
	Reviewers  []string
	Labels     []string
	Projects   []string
	Milestones []string
}

// RepoResolveMetadataIDs looks up GraphQL node IDs in bulk
func RepoResolveMetadataIDs(client *Client, repo ghrepo.Interface, input RepoResolveInput) (*RepoMetadataResult, error) {
	users := input.Assignees
	hasUser := func(target string) bool {
		for _, u := range users {
			if strings.EqualFold(u, target) {
				return true
			}
		}
		return false
	}

	var teams []string
	for _, r := range input.Reviewers {
		if i := strings.IndexRune(r, '/'); i > -1 {
			teams = append(teams, r[i+1:])
		} else if !hasUser(r) {
			users = append(users, r)
		}
	}

	// there is no way to look up projects nor milestones by name, so preload them all
	mi := RepoMetadataInput{
		Projects:   len(input.Projects) > 0,
		Milestones: len(input.Milestones) > 0,
	}
	result, err := RepoMetadata(client, repo, mi)
	if err != nil {
		return result, err
	}
	if len(users) == 0 && len(teams) == 0 && len(input.Labels) == 0 {
		return result, nil
	}

	query := &bytes.Buffer{}
	fmt.Fprint(query, "query RepositoryResolveMetadataIDs {\n")
	for i, u := range users {
		fmt.Fprintf(query, "u%03d: user(login:%q){id,login}\n", i, u)
	}
	if len(input.Labels) > 0 {
		fmt.Fprintf(query, "repository(owner:%q,name:%q){\n", repo.RepoOwner(), repo.RepoName())
		for i, l := range input.Labels {
			fmt.Fprintf(query, "l%03d: label(name:%q){id,name}\n", i, l)
		}
		fmt.Fprint(query, "}\n")
	}
	if len(teams) > 0 {
		fmt.Fprintf(query, "organization(login:%q){\n", repo.RepoOwner())
		for i, t := range teams {
			fmt.Fprintf(query, "t%03d: team(slug:%q){id,slug}\n", i, t)
		}
		fmt.Fprint(query, "}\n")
	}
	fmt.Fprint(query, "}\n")

	response := make(map[string]json.RawMessage)
	err = client.GraphQL(repo.RepoHost(), query.String(), nil, &response)
	if err != nil {
		return result, err
	}

	for key, v := range response {
		switch key {
		case "repository":
			repoResponse := make(map[string]RepoLabel)
			err := json.Unmarshal(v, &repoResponse)
			if err != nil {
				return result, err
			}
			for _, l := range repoResponse {
				result.Labels = append(result.Labels, l)
			}
		case "organization":
			orgResponse := make(map[string]OrgTeam)
			err := json.Unmarshal(v, &orgResponse)
			if err != nil {
				return result, err
			}
			for _, t := range orgResponse {
				result.Teams = append(result.Teams, t)
			}
		default:
			user := RepoAssignee{}
			err := json.Unmarshal(v, &user)
			if err != nil {
				return result, err
			}
			result.AssignableUsers = append(result.AssignableUsers, user)
		}
	}

	return result, nil
}

type RepoProject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Number       int    `json:"number"`
	ResourcePath string `json:"resourcePath"`
}

// RepoProjects fetches all open projects for a repository
func RepoProjects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	type responseData struct {
		Repository struct {
			Projects struct {
				Nodes    []RepoProject
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"projects(states: [OPEN], first: 100, orderBy: {field: NAME, direction: ASC}, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var projects []RepoProject
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "RepositoryProjectList", &query, variables)
		if err != nil {
			return nil, err
		}

		projects = append(projects, query.Repository.Projects.Nodes...)
		if !query.Repository.Projects.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.Projects.PageInfo.EndCursor)
	}

	return projects, nil
}

// RepoAndOrgProjects fetches all open projects for a repository and its org
func RepoAndOrgProjects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	projects, err := RepoProjects(client, repo)
	if err != nil {
		return projects, fmt.Errorf("error fetching projects: %w", err)
	}

	orgProjects, err := OrganizationProjects(client, repo)
	// TODO: better detection of non-org repos
	if err != nil && !strings.HasPrefix(err.Error(), "Could not resolve to an Organization") {
		return projects, fmt.Errorf("error fetching organization projects: %w", err)
	}
	projects = append(projects, orgProjects...)

	return projects, nil
}

type RepoAssignee struct {
	ID    string
	Login string
}

// RepoAssignableUsers fetches all the assignable users for a repository
func RepoAssignableUsers(client *Client, repo ghrepo.Interface) ([]RepoAssignee, error) {
	type responseData struct {
		Repository struct {
			AssignableUsers struct {
				Nodes    []RepoAssignee
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"assignableUsers(first: 100, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var users []RepoAssignee
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "RepositoryAssignableUsers", &query, variables)
		if err != nil {
			return nil, err
		}

		users = append(users, query.Repository.AssignableUsers.Nodes...)
		if !query.Repository.AssignableUsers.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.AssignableUsers.PageInfo.EndCursor)
	}

	return users, nil
}

type RepoLabel struct {
	ID   string
	Name string
}

// RepoLabels fetches all the labels in a repository
func RepoLabels(client *Client, repo ghrepo.Interface) ([]RepoLabel, error) {
	type responseData struct {
		Repository struct {
			Labels struct {
				Nodes    []RepoLabel
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"labels(first: 100, orderBy: {field: NAME, direction: ASC}, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var labels []RepoLabel
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "RepositoryLabelList", &query, variables)
		if err != nil {
			return nil, err
		}

		labels = append(labels, query.Repository.Labels.Nodes...)
		if !query.Repository.Labels.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.Labels.PageInfo.EndCursor)
	}

	return labels, nil
}

type RepoMilestone struct {
	ID    string
	Title string
}

// RepoMilestones fetches milestones in a repository
func RepoMilestones(client *Client, repo ghrepo.Interface, state string) ([]RepoMilestone, error) {
	type responseData struct {
		Repository struct {
			Milestones struct {
				Nodes    []RepoMilestone
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"milestones(states: $states, first: 100, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	var states []githubv4.MilestoneState
	switch state {
	case "open":
		states = []githubv4.MilestoneState{"OPEN"}
	case "closed":
		states = []githubv4.MilestoneState{"CLOSED"}
	case "all":
		states = []githubv4.MilestoneState{"OPEN", "CLOSED"}
	default:
		return nil, fmt.Errorf("invalid state: %s", state)
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"states":    states,
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var milestones []RepoMilestone
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "RepositoryMilestoneList", &query, variables)
		if err != nil {
			return nil, err
		}

		milestones = append(milestones, query.Repository.Milestones.Nodes...)
		if !query.Repository.Milestones.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Repository.Milestones.PageInfo.EndCursor)
	}

	return milestones, nil
}

func MilestoneByTitle(client *Client, repo ghrepo.Interface, state, title string) (*RepoMilestone, error) {
	milestones, err := RepoMilestones(client, repo, state)
	if err != nil {
		return nil, err
	}

	for i := range milestones {
		if strings.EqualFold(milestones[i].Title, title) {
			return &milestones[i], nil
		}
	}
	return nil, fmt.Errorf("no milestone found with title %q", title)
}

func MilestoneByNumber(client *Client, repo ghrepo.Interface, number int32) (*RepoMilestone, error) {
	var query struct {
		Repository struct {
			Milestone *RepoMilestone `graphql:"milestone(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(repo.RepoOwner()),
		"name":   githubv4.String(repo.RepoName()),
		"number": githubv4.Int(number),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	err := gql.QueryNamed(context.Background(), "RepositoryMilestoneByNumber", &query, variables)
	if err != nil {
		return nil, err
	}
	if query.Repository.Milestone == nil {
		return nil, fmt.Errorf("no milestone found with number '%d'", number)
	}

	return query.Repository.Milestone, nil
}

func ProjectNamesToPaths(client *Client, repo ghrepo.Interface, projectNames []string) ([]string, error) {
	var paths []string
	projects, err := RepoAndOrgProjects(client, repo)
	if err != nil {
		return paths, err
	}
	return ProjectsToPaths(projects, projectNames)
}

func CreateRepoTransformToV4(apiClient *Client, hostname string, method string, path string, body io.Reader) (*Repository, error) {
	var responsev3 repositoryV3
	err := apiClient.REST(hostname, method, path, body, &responsev3)

	if err != nil {
		return nil, err
	}

	return &Repository{
		Name:      responsev3.Name,
		CreatedAt: responsev3.CreatedAt,
		Owner: RepositoryOwner{
			Login: responsev3.Owner.Login,
		},
		ID:        responsev3.NodeID,
		hostname:  hostname,
		URL:       responsev3.HTMLUrl,
		IsPrivate: responsev3.Private,
	}, nil
}
