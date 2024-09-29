package queries

import (
	"net/http"

	"github.com/cli/cli/v2/api"
)

// ListLicenseTemplates fetches available repository license templates.
// It uses API v3 because license template isn't supported by GraphQL.
func ListLicenseTemplates(httpClient *http.Client, hostname string) ([]api.License, error) {
	var licenseTemplates []api.License
	client := api.NewClientFromHTTP(httpClient)
	err := client.REST(hostname, "GET", "licenses", nil, &licenseTemplates)
	if err != nil {
		return nil, err
	}
	return licenseTemplates, nil
}

// ListGitIgnoreTemplates fetches available repository gitignore templates.
// It uses API v3 here because gitignore template isn't supported by GraphQL.
func ListGitIgnoreTemplates(httpClient *http.Client, hostname string) ([]string, error) {
	var gitIgnoreTemplates []string
	client := api.NewClientFromHTTP(httpClient)
	err := client.REST(hostname, "GET", "gitignore/templates", nil, &gitIgnoreTemplates)
	if err != nil {
		return []string{}, err
	}
	return gitIgnoreTemplates, nil
}
