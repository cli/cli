package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/api"
)

// repoCreateInput represents input parameters for repoCreate
type repoCreateInput struct {
	Name        string `json:"name"`
	Visibility  string `json:"visibility"`
	HomepageURL string `json:"homepageUrl,omitempty"`
	Description string `json:"description,omitempty"`

	OwnerID string `json:"ownerId,omitempty"`
	TeamID  string `json:"teamId,omitempty"`

	HasIssuesEnabled  bool   `json:"hasIssuesEnabled"`
	HasWikiEnabled    bool   `json:"hasWikiEnabled"`
	GitIgnoreTemplate string `json:"gitignore_template,omitempty"`
	LicenseTemplate   string `json:"license_template,omitempty"`
}

type repoTemplateInput struct {
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
	OwnerID    string `json:"ownerId,omitempty"`

	RepositoryID string `json:"repositoryId,omitempty"`
	Description  string `json:"description,omitempty"`
}

// repoCreate creates a new GitHub repository
func repoCreate(client *http.Client, hostname string, input repoCreateInput, templateRepositoryID string) (*api.Repository, error) {
	apiClient := api.NewClientFromHTTP(client)

	ownerName := input.OwnerID
	isOrg := false

	if input.TeamID != "" {
		orgID, teamID, err := resolveOrganizationTeam(apiClient, hostname, input.OwnerID, input.TeamID)
		if err != nil {
			return nil, err
		}
		input.TeamID = teamID
		input.OwnerID = orgID
	} else if input.OwnerID != "" {
		var orgID string
		var err error
		orgID, isOrg, err = resolveOrganization(apiClient, hostname, input.OwnerID)
		if err != nil {
			return nil, err
		}
		input.OwnerID = orgID
	}

	if templateRepositoryID != "" {
		var response struct {
			CloneTemplateRepository struct {
				Repository api.Repository
			}
		}

		if input.OwnerID == "" {
			var err error
			input.OwnerID, err = api.CurrentUserID(apiClient, hostname)
			if err != nil {
				return nil, err
			}
		}

		templateInput := repoTemplateInput{
			Name:         input.Name,
			Visibility:   input.Visibility,
			OwnerID:      input.OwnerID,
			RepositoryID: templateRepositoryID,
		}

		variables := map[string]interface{}{
			"input": templateInput,
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

		return api.InitRepoHostname(&response.CloneTemplateRepository.Repository, hostname), nil
	}

	var response struct {
		CreateRepository struct {
			Repository api.Repository
		}
	}

	variables := map[string]interface{}{
		"input": input,
	}

	if input.GitIgnoreTemplate != "" || input.LicenseTemplate != "" {
		input.Visibility = strings.ToLower(input.Visibility)
		body := &bytes.Buffer{}
		enc := json.NewEncoder(body)
		if err := enc.Encode(input); err != nil {
			return nil, err
		}

		path := "user/repos"
		if isOrg {
			path = fmt.Sprintf("orgs/%s/repos", ownerName)
		}

		repo, err := api.CreateRepoTransformToV4(apiClient, hostname, "POST", path, body)
		if err != nil {
			return nil, err
		}
		return api.InitRepoHostname(repo, hostname), nil
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

// using API v3 here because the equivalent in GraphQL needs `read:org` scope
func resolveOrganization(client *api.Client, hostname, orgName string) (string, bool, error) {
	var response struct {
		NodeID string `json:"node_id"`
		Type   string
	}
	err := client.REST(hostname, "GET", fmt.Sprintf("users/%s", orgName), nil, &response)
	return response.NodeID, response.Type == "Organization", err
}

// using API v3 here because the equivalent in GraphQL needs `read:org` scope
func resolveOrganizationTeam(client *api.Client, hostname, orgName, teamSlug string) (string, string, error) {
	var response struct {
		NodeID       string `json:"node_id"`
		Organization struct {
			NodeID string `json:"node_id"`
		}
	}
	err := client.REST(hostname, "GET", fmt.Sprintf("orgs/%s/teams/%s", orgName, teamSlug), nil, &response)
	return response.Organization.NodeID, response.NodeID, err
}

// ListGitIgnoreTemplates uses API v3 here because gitignore template isn't supported by GraphQL yet.
func ListGitIgnoreTemplates(client *api.Client, hostname string) ([]string, error) {
	var gitIgnoreTemplates []string
	err := client.REST(hostname, "GET", "gitignore/templates", nil, &gitIgnoreTemplates)
	if err != nil {
		return []string{}, err
	}
	return gitIgnoreTemplates, nil
}

// ListLicenseTemplates uses API v3 here because license template isn't supported by GraphQL yet.
func ListLicenseTemplates(client *api.Client, hostname string) ([]api.License, error) {
	var licenseTemplates []api.License
	err := client.REST(hostname, "GET", "licenses", nil, &licenseTemplates)
	if err != nil {
		return nil, err
	}
	return licenseTemplates, nil
}
