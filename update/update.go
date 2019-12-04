package update

import (
	"fmt"

	"github.com/github/gh-cli/api"
	"github.com/hashicorp/go-version"
)

// ReleaseInfo stores information about a release
type ReleaseInfo struct {
	Version string `json:"tag_name"`
	URL     string `json:"html_url"`
}

// CheckForUpdate checks whether this software has had a newer relase on GitHub
func CheckForUpdate(client *api.Client, repo, currentVersion string) (*ReleaseInfo, error) {
	latestRelease := ReleaseInfo{}
	err := client.REST("GET", fmt.Sprintf("repos/%s/releases/latest", repo), nil, &latestRelease)
	if err != nil {
		return nil, err
	}

	if versionGreaterThan(latestRelease.Version, currentVersion) {
		return &latestRelease, nil
	}

	return nil, nil
}

func versionGreaterThan(v, w string) bool {
	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}
