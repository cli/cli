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
