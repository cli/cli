package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
	"github.com/shurcooL/githubv4"
)

// Repository contains information about a GitHub repo
type Repository struct {
	ID          string
	Name        string
	Description string
	URL         string
	CloneURL    string
	CreatedAt   time.Time
	Owner       RepositoryOwner

	IsPrivate        bool
	HasIssuesEnabled bool
	ViewerPermission string
	DefaultBranchRef struct {
		Name string
	}

	Parent *Repository
}

// RepositoryOwner is the owner of a GitHub repository
type RepositoryOwner struct {
	Login string
}

// RepoOwner is the login name of the owner
func (r Repository) RepoOwner() string {
	return r.Owner.Login
}

// RepoName is the name of the repository
func (r Repository) RepoName() string {
	return r.Name
}

// IsFork is true when this repository has a parent repository
func (r Repository) IsFork() bool {
	return r.Parent != nil
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

func GitHubRepo(client *Client, repo ghrepo.Interface) (*Repository, error) {
	query := `
	query($owner: String!, $name: String!) {
		repository(owner: $owner, name: $name) {
			id
			hasIssuesEnabled
			description
			viewerPermission
			defaultBranchRef {
				name
			}
		}
	}`
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"name":  repo.RepoName(),
	}

	result := struct {
		Repository Repository
	}{}
	err := client.GraphQL(query, variables, &result)

	if err != nil {
		return nil, err
	}

	return &result.Repository, nil
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

	v4 := githubv4.NewClient(client.http)
	err := v4.Query(context.Background(), &query, variables)
	if err != nil {
		return nil, err
	}
	if query.Repository.Parent == nil {
		return nil, nil
	}

	parent := ghrepo.New(query.Repository.Parent.Owner.Login, query.Repository.Parent.Name)
	return parent, nil
}

// RepoNetworkResult describes the relationship between related repositories
type RepoNetworkResult struct {
	ViewerLogin  string
	Repositories []*Repository
}

// RepoNetwork inspects the relationship between multiple GitHub repositories
func RepoNetwork(client *Client, repos []ghrepo.Interface) (RepoNetworkResult, error) {
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

	err := client.GraphQL(fmt.Sprintf(`
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
	query {
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
			result.Repositories = append(result.Repositories, &repo)
		} else {
			return result, fmt.Errorf("unknown GraphQL result key %q", name)
		}
	}
	return result, nil
}

// repositoryV3 is the repository result from GitHub API v3
type repositoryV3 struct {
	NodeID    string
	Name      string
	CreatedAt time.Time `json:"created_at"`
	CloneURL  string    `json:"clone_url"`
	Owner     struct {
		Login string
	}
}

// ForkRepo forks the repository on GitHub and returns the new repository
func ForkRepo(client *Client, repo ghrepo.Interface) (*Repository, error) {
	path := fmt.Sprintf("repos/%s/forks", ghrepo.FullName(repo))
	body := bytes.NewBufferString(`{}`)
	result := repositoryV3{}
	err := client.REST("POST", path, body, &result)
	if err != nil {
		return nil, err
	}

	return &Repository{
		ID:        result.NodeID,
		Name:      result.Name,
		CloneURL:  result.CloneURL,
		CreatedAt: result.CreatedAt,
		Owner: RepositoryOwner{
			Login: result.Owner.Login,
		},
		ViewerPermission: "WRITE",
	}, nil
}

// RepoFindFork finds a fork of repo affiliated with the viewer
func RepoFindFork(client *Client, repo ghrepo.Interface) (*Repository, error) {
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
	}

	if err := client.GraphQL(`
	query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			forks(first: 1, affiliations: [OWNER, COLLABORATOR]) {
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

	forks := result.Repository.Forks.Nodes
	// we check ViewerCanPush, even though we expect it to always be true per
	// `affiliations` condition, to guard against versions of GitHub with a
	// faulty `affiliations` implementation
	if len(forks) > 0 && forks[0].ViewerCanPush() {
		return &forks[0], nil
	}
	return nil, &NotFoundError{errors.New("no fork found")}
}

// RepoCreateInput represents input parameters for RepoCreate
type RepoCreateInput struct {
	Name        string `json:"name"`
	Visibility  string `json:"visibility"`
	HomepageURL string `json:"homepageUrl,omitempty"`
	Description string `json:"description,omitempty"`

	OwnerID string `json:"ownerId,omitempty"`
	TeamID  string `json:"teamId,omitempty"`

	HasIssuesEnabled bool `json:"hasIssuesEnabled"`
	HasWikiEnabled   bool `json:"hasWikiEnabled"`
}

// RepoCreate creates a new GitHub repository
func RepoCreate(client *Client, input RepoCreateInput) (*Repository, error) {
	var response struct {
		CreateRepository struct {
			Repository Repository
		}
	}

	if input.TeamID != "" {
		orgID, teamID, err := resolveOrganizationTeam(client, input.OwnerID, input.TeamID)
		if err != nil {
			return nil, err
		}
		input.TeamID = teamID
		input.OwnerID = orgID
	} else if input.OwnerID != "" {
		orgID, err := resolveOrganization(client, input.OwnerID)
		if err != nil {
			return nil, err
		}
		input.OwnerID = orgID
	}

	variables := map[string]interface{}{
		"input": input,
	}

	err := client.GraphQL(`
	mutation($input: CreateRepositoryInput!) {
		createRepository(input: $input) {
			repository {
				id
				name
				owner { login }
				url
			}
		}
	}
	`, variables, &response)
	if err != nil {
		return nil, err
	}

	return &response.CreateRepository.Repository, nil
}

func RepositoryReadme(client *Client, fullName string) (string, error) {
	type readmeResponse struct {
		Name    string
		Content string
	}

	var readme readmeResponse

	err := client.REST("GET", fmt.Sprintf("repos/%s/readme", fullName), nil, &readme)
	if err != nil && !strings.HasSuffix(err.Error(), "'Not Found'") {
		return "", fmt.Errorf("could not get readme for repo: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(readme.Content)
	if err != nil {
		return "", fmt.Errorf("failed to decode readme: %w", err)
	}

	readmeContent := string(decoded)

	if isMarkdownFile(readme.Name) {
		readmeContent, err = utils.RenderMarkdown(readmeContent)
		if err != nil {
			return "", fmt.Errorf("failed to render readme as markdown: %w", err)
		}
	}

	return readmeContent, nil

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

func (m *RepoMetadataResult) MilestoneToID(title string) (string, error) {
	for _, m := range m.Milestones {
		if strings.EqualFold(title, m.Title) {
			return m.ID, nil
		}
	}
	return "", errors.New("not found")
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
			teams, err := OrganizationTeams(client, repo.RepoOwner())
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
			projects, err := RepoProjects(client, repo)
			if err != nil {
				errc <- fmt.Errorf("error fetching projects: %w", err)
				return
			}
			result.Projects = projects

			orgProjects, err := OrganizationProjects(client, repo.RepoOwner())
			// TODO: better detection of non-org repos
			if err != nil && !strings.HasPrefix(err.Error(), "Could not resolve to an Organization") {
				errc <- fmt.Errorf("error fetching organization projects: %w", err)
				return
			}
			result.Projects = append(result.Projects, orgProjects...)
			errc <- nil
		}()
	}
	if input.Milestones {
		count++
		go func() {
			milestones, err := RepoMilestones(client, repo)
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
	fmt.Fprint(query, "{\n")
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
	err = client.GraphQL(query.String(), nil, &response)
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
	ID   string
	Name string
}

// RepoProjects fetches all open projects for a repository
func RepoProjects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	var query struct {
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

	v4 := githubv4.NewClient(client.http)

	var projects []RepoProject
	for {
		err := v4.Query(context.Background(), &query, variables)
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

type RepoAssignee struct {
	ID    string
	Login string
}

// RepoAssignableUsers fetches all the assignable users for a repository
func RepoAssignableUsers(client *Client, repo ghrepo.Interface) ([]RepoAssignee, error) {
	var query struct {
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

	v4 := githubv4.NewClient(client.http)

	var users []RepoAssignee
	for {
		err := v4.Query(context.Background(), &query, variables)
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
	var query struct {
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

	v4 := githubv4.NewClient(client.http)

	var labels []RepoLabel
	for {
		err := v4.Query(context.Background(), &query, variables)
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

// RepoMilestones fetches all open milestones in a repository
func RepoMilestones(client *Client, repo ghrepo.Interface) ([]RepoMilestone, error) {
	var query struct {
		Repository struct {
			Milestones struct {
				Nodes    []RepoMilestone
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"milestones(states: [OPEN], first: 100, after: $endCursor)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"name":      githubv4.String(repo.RepoName()),
		"endCursor": (*githubv4.String)(nil),
	}

	v4 := githubv4.NewClient(client.http)

	var milestones []RepoMilestone
	for {
		err := v4.Query(context.Background(), &query, variables)
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

func isMarkdownFile(filename string) bool {
	// kind of gross, but i'm assuming that 90% of the time the suffix will just be .md. it didn't
	// seem worth executing a regex for this given that assumption.
	return strings.HasSuffix(filename, ".md") ||
		strings.HasSuffix(filename, ".markdown") ||
		strings.HasSuffix(filename, ".mdown") ||
		strings.HasSuffix(filename, ".mkdown")
}
