package extension

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/api"
)

// extensionCreateInput is input parameters for the extensionCreate method
type extensionCreateInput struct {
	Name         string
	Description  string
	Visibility   string
	Organization string
	RepositoryID string
}

// cloneTemplateRepositoryInput is the payload for creating a repo from a template using GraphQL
type cloneTemplateRepositoryInput struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Visibility   string `json:"visibility"`
	OwnerID      string `json:"ownerId"`
	RepositoryID string `json:"repositoryId"`
}

func extensionCreate(client *http.Client, hostname string, input extensionCreateInput) (*api.Repository, error) {
	var ownerID string
	apiClient := api.NewClientFromHTTP(client)

	var response struct {
		CloneTemplateRepository struct {
			Repository api.Repository
		}
	}

	if input.Organization != "" {
		var err error
		ownerID, err = resolveOrganizationID(apiClient, hostname, input.Organization)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		ownerID, err = api.CurrentUserID(apiClient, hostname)
		if err != nil {
			return nil, err
		}
	}

	variables := map[string]interface{}{
		"input": cloneTemplateRepositoryInput{
			Name:         input.Name,
			Description:  input.Description,
			Visibility:   strings.ToUpper(input.Visibility),
			OwnerID:      ownerID,
			RepositoryID: input.RepositoryID,
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
				sshUrl
			}
		}
	}
	`, variables, &response)
	if err != nil {
		return nil, err
	}

	return api.InitRepoHostname(&response.CloneTemplateRepository.Repository, hostname), nil
}

func resolveOrganizationID(client *api.Client, hostname, organization string) (string, error) {
	var response struct {
		NodeID string `json:"node_id"`
	}
	err := client.REST(hostname, "GET", fmt.Sprintf("users/%s", organization), nil, &response)
	return response.NodeID, err
}
