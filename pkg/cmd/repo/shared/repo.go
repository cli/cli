package shared

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/api"
)

var invalidCharactersRE = regexp.MustCompile(`[^\w._-]+`)

// NormalizeRepoName takes in the repo name the user inputted and normalizes it using the same logic as GitHub (GitHub.com/new)
func NormalizeRepoName(repoName string) string {
	newName := invalidCharactersRE.ReplaceAllString(repoName, "-")
	return strings.TrimSuffix(newName, ".git")
}

// ListLicenseTemplates uses API v3 here because license template isn't supported by GraphQL yet.
func ListLicenseTemplates(httpClient *http.Client, hostname string) ([]api.License, error) {
	var licenseTemplates []api.License
	client := api.NewClientFromHTTP(httpClient)
	err := client.REST(hostname, "GET", "licenses", nil, &licenseTemplates)
	if err != nil {
		return nil, err
	}
	return licenseTemplates, nil
}
