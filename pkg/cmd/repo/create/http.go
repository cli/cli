package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/shurcooL/githubv4"
)

// repoCreateInput is input parameters for the repoCreate method
type repoCreateInput struct {
	Name                 string
	HomepageURL          string
	Description          string
	Visibility           string
	OwnerLogin           string
	TeamSlug             string
	TemplateRepositoryID string
	HasIssuesEnabled     bool
	HasWikiEnabled       bool
	GitIgnoreTemplate    string
	LicenseTemplate      string
	IncludeAllBranches   bool
	InitReadme           bool
}

// createRepositoryInputV3 is the payload for the repo create REST API
type createRepositoryInputV3 struct {
	Name              string `json:"name"`
	HomepageURL       string `json:"homepage,omitempty"`
	Description       string `json:"description,omitempty"`
	IsPrivate         bool   `json:"private"`
	Visibility        string `json:"visibility,omitempty"`
	TeamID            uint64 `json:"team_id,omitempty"`
	HasIssuesEnabled  bool   `json:"has_issues"`
	HasWikiEnabled    bool   `json:"has_wiki"`
	GitIgnoreTemplate string `json:"gitignore_template,omitempty"`
	LicenseTemplate   string `json:"license_template,omitempty"`
	InitReadme        bool   `json:"auto_init,omitempty"`
}

// createRepositoryInput is the payload for the repo create GraphQL mutation
type createRepositoryInput struct {
	Name             string `json:"name"`
	HomepageURL      string `json:"homepageUrl,omitempty"`
	Description      string `json:"description,omitempty"`
	Visibility       string `json:"visibility"`
	OwnerID          string `json:"ownerId,omitempty"`
	TeamID           string `json:"teamId,omitempty"`
	HasIssuesEnabled bool   `json:"hasIssuesEnabled"`
	HasWikiEnabled   bool   `json:"hasWikiEnabled"`
}

// cloneTemplateRepositoryInput is the payload for creating a repo from a template using GraphQL
type cloneTemplateRepositoryInput struct {
	Name               string `json:"name"`
	Visibility         string `json:"visibility"`
	Description        string `json:"description,omitempty"`
	OwnerID            string `json:"ownerId"`
	RepositoryID       string `json:"repositoryId"`
	IncludeAllBranches bool   `json:"includeAllBranches"`
}

type updateRepositoryInput struct {
	RepositoryID     string `json:"repositoryId"`
	HasWikiEnabled   bool   `json:"hasWikiEnabled"`
	HasIssuesEnabled bool   `json:"hasIssuesEnabled"`
	HomepageURL      string `json:"homepageUrl,omitempty"`
}

// repoCreate creates a new GitHub repository
func repoCreate(client *http.Client, hostname string, input repoCreateInput) (*api.Repository, error) {
	isOrg := false
	var ownerID string
	var teamID string
	var teamIDv3 uint64

	apiClient := api.NewClientFromHTTP(client)

	if input.TeamSlug != "" {
		team, err := resolveOrganizationTeam(apiClient, hostname, input.OwnerLogin, input.TeamSlug)
		if err != nil {
			return nil, err
		}
		teamIDv3 = team.ID
		teamID = team.NodeID
		ownerID = team.Organization.NodeID
		isOrg = true
	} else if input.OwnerLogin != "" {
		owner, err := resolveOwner(apiClient, hostname, input.OwnerLogin)
		if err != nil {
			return nil, err
		}
		ownerID = owner.NodeID
		isOrg = owner.IsOrganization()
	}

	isInternal := strings.ToLower(input.Visibility) == "internal"
	if isInternal && !isOrg {
		return nil, fmt.Errorf("internal repositories can only be created within an organization")
	}

	if input.TemplateRepositoryID != "" {
		var response struct {
			CloneTemplateRepository struct {
				Repository api.Repository
			}
		}

		if ownerID == "" {
			var err error
			ownerID, err = api.CurrentUserID(apiClient, hostname)
			if err != nil {
				return nil, err
			}
		}

		variables := map[string]interface{}{
			"input": cloneTemplateRepositoryInput{
				Name:               input.Name,
				Description:        input.Description,
				Visibility:         strings.ToUpper(input.Visibility),
				OwnerID:            ownerID,
				RepositoryID:       input.TemplateRepositoryID,
				IncludeAllBranches: input.IncludeAllBranches,
			},
		}

		err := apiClient.GraphQL(hostname, `
		mutation CloneTemplateRepository($input: CloneTemplateRepositoryInput!) {
			cloneTemplateRepository(input: $input) {
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

		if !input.HasWikiEnabled || !input.HasIssuesEnabled || input.HomepageURL != "" {
			updateVariables := map[string]interface{}{
				"input": updateRepositoryInput{
					RepositoryID:     response.CloneTemplateRepository.Repository.ID,
					HasWikiEnabled:   input.HasWikiEnabled,
					HasIssuesEnabled: input.HasIssuesEnabled,
					HomepageURL:      input.HomepageURL,
				},
			}

			if err := apiClient.GraphQL(hostname, `
				mutation UpdateRepository($input: UpdateRepositoryInput!) {
					updateRepository(input: $input) {
						repository {
							id
						}
					}
				}
			`, updateVariables, nil); err != nil {
				return nil, err
			}
		}

		return api.InitRepoHostname(&response.CloneTemplateRepository.Repository, hostname), nil
	}

	if input.GitIgnoreTemplate != "" || input.LicenseTemplate != "" || input.InitReadme {
		inputv3 := createRepositoryInputV3{
			Name:              input.Name,
			HomepageURL:       input.HomepageURL,
			Description:       input.Description,
			IsPrivate:         strings.EqualFold(input.Visibility, "PRIVATE"),
			TeamID:            teamIDv3,
			HasIssuesEnabled:  input.HasIssuesEnabled,
			HasWikiEnabled:    input.HasWikiEnabled,
			GitIgnoreTemplate: input.GitIgnoreTemplate,
			LicenseTemplate:   input.LicenseTemplate,
			InitReadme:        input.InitReadme,
		}

		path := "user/repos"
		if isOrg {
			path = fmt.Sprintf("orgs/%s/repos", input.OwnerLogin)
			inputv3.Visibility = strings.ToLower(input.Visibility)
		}

		body := &bytes.Buffer{}
		enc := json.NewEncoder(body)
		if err := enc.Encode(inputv3); err != nil {
			return nil, err
		}

		repo, err := api.CreateRepoTransformToV4(apiClient, hostname, "POST", path, body)
		if err != nil {
			return nil, err
		}
		return repo, nil
	}

	var response struct {
		CreateRepository struct {
			Repository api.Repository
		}
	}

	variables := map[string]interface{}{
		"input": createRepositoryInput{
			Name:             input.Name,
			Description:      input.Description,
			HomepageURL:      input.HomepageURL,
			Visibility:       strings.ToUpper(input.Visibility),
			OwnerID:          ownerID,
			TeamID:           teamID,
			HasIssuesEnabled: input.HasIssuesEnabled,
			HasWikiEnabled:   input.HasWikiEnabled,
		},
	}

	err := apiClient.GraphQL(hostname, `
	mutation RepositoryCreate($input: CreateRepositoryInput!) {
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

	return api.InitRepoHostname(&response.CreateRepository.Repository, hostname), nil
}

type ownerResponse struct {
	NodeID string `json:"node_id"`
	Type   string `json:"type"`
}

func (r *ownerResponse) IsOrganization() bool {
	return r.Type == "Organization"
}

func resolveOwner(client *api.Client, hostname, orgName string) (*ownerResponse, error) {
	var response ownerResponse
	err := client.REST(hostname, "GET", fmt.Sprintf("users/%s", orgName), nil, &response)
	return &response, err
}

type teamResponse struct {
	ID           uint64 `json:"id"`
	NodeID       string `json:"node_id"`
	Organization struct {
		NodeID string `json:"node_id"`
	}
}

func resolveOrganizationTeam(client *api.Client, hostname, orgName, teamSlug string) (*teamResponse, error) {
	var response teamResponse
	err := client.REST(hostname, "GET", fmt.Sprintf("orgs/%s/teams/%s", orgName, teamSlug), nil, &response)
	return &response, err
}

func listTemplateRepositories(client *http.Client, hostname, owner string) ([]api.Repository, error) {
	ownerConnection := "repositoryOwner(login: $owner)"

	variables := map[string]interface{}{
		"perPage": githubv4.Int(100),
		"owner":   githubv4.String(owner),
	}
	inputs := []string{"$perPage:Int!", "$endCursor:String", "$owner:String!"}

	type result struct {
		RepositoryOwner struct {
			Login        string
			Repositories struct {
				Nodes      []api.Repository
				TotalCount int
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
			}
		}
	}

	query := fmt.Sprintf(`query RepositoryList(%s) {
		%s {
			login
			repositories(first: $perPage, after: $endCursor, ownerAffiliations: OWNER, orderBy: { field: PUSHED_AT, direction: DESC }) {
				nodes{
					id
					name
					isTemplate
					defaultBranchRef {
						name
					}
				}
				totalCount
				pageInfo{hasNextPage,endCursor}
			}
		}
	}`, strings.Join(inputs, ","), ownerConnection)

	apiClient := api.NewClientFromHTTP(client)
	var templateRepositories []api.Repository
	for {
		var res result
		err := apiClient.GraphQL(hostname, query, variables, &res)
		if err != nil {
			return nil, err
		}

		owner := res.RepositoryOwner

		for _, repo := range owner.Repositories.Nodes {
			if repo.IsTemplate {
				templateRepositories = append(templateRepositories, repo)
			}
		}

		if !owner.Repositories.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(owner.Repositories.PageInfo.EndCursor)
	}

	return templateRepositories, nil
}

// Returns the current username and any orgs that user is a member of.
func userAndOrgs(httpClient *http.Client, hostname string) (string, []string, error) {
	client := api.NewClientFromHTTP(httpClient)
	return api.CurrentLoginNameAndOrgs(client, hostname)
}
