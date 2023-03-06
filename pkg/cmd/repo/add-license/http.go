package add_license

import (
	"net/http"

	"github.com/cli/cli/v2/api"
)

// duplicated from repo-create

// listLicenseTemplates uses API v3 here because license template isn't supported by GraphQL yet.
func listLicenseTemplates(httpClient *http.Client, hostname string) ([]api.License, error) {
	var licenseTemplates []api.License
	client := api.NewClientFromHTTP(httpClient)
	err := client.REST(hostname, "GET", "licenses", nil, &licenseTemplates)
	if err != nil {
		return nil, err
	}
	return licenseTemplates, nil
}

func interactiveLicense(client *http.Client, hostname string, prompter iprompter) (string, error) {
	licenses, err := listLicenseTemplates(client, hostname)
	if err != nil {
		return "", err
	}
	licenseNames := make([]string, 0, len(licenses))
	licenseNames = append(licenseNames, "Open 'choosealicense.com' in browser to view information about licenses")
	for _, license := range licenses {
		licenseNames = append(licenseNames, license.Name)
	}

	selected, err := prompter.Select("Choose a license", "", licenseNames)
	if err != nil {
		return "", err
	}
	if selected == 0 {
		return "", nil
	}
	return licenses[selected].Key, nil
}
