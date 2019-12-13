package update

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/github/gh-cli/api"
	"github.com/hashicorp/go-version"
	"gopkg.in/yaml.v3"
)

// ReleaseInfo stores information about a release
type ReleaseInfo struct {
	Version string `json:"tag_name"`
	URL     string `json:"html_url"`
}

type StateEntry struct {
	CheckedForUpdateAt time.Time   `yaml:"checked_for_update_at"`
	LatestRelease      ReleaseInfo `yaml:"latest_release"`
}

// CheckForUpdate checks whether this software has had a newer relase on GitHub
func CheckForUpdate(client *api.Client, stateFilePath, repo, currentVersion string) (*ReleaseInfo, error) {
	latestRelease, err := getLatestReleaseInfo(client, stateFilePath, repo, currentVersion)
	if err != nil {
		return nil, err
	}

	if versionGreaterThan(latestRelease.Version, currentVersion) {
		return latestRelease, nil
	}

	return nil, nil
}

func getLatestReleaseInfo(client *api.Client, stateFilePath, repo, currentVersion string) (*ReleaseInfo, error) {
	stateEntry, err := getStateEntry(stateFilePath)
	if err == nil && time.Since(stateEntry.CheckedForUpdateAt).Hours() < 24 {
		return &stateEntry.LatestRelease, nil
	}

	latestRelease := ReleaseInfo{}
	err = client.REST("GET", fmt.Sprintf("repos/%s/releases/latest", repo), nil, &latestRelease)
	if err != nil {
		return nil, err
	}

	err = setStateEntry(stateFilePath, time.Now(), latestRelease)
	if err != nil {
		return nil, err
	}

	return &latestRelease, nil
}

func getStateEntry(stateFilePath string) (*StateEntry, error) {
	content, err := ioutil.ReadFile(stateFilePath)
	if err != nil {
		return nil, err
	}

	var stateEntry StateEntry
	err = yaml.Unmarshal(content, &stateEntry)
	if err != nil {
		return nil, err
	}

	return &stateEntry, nil
}

func setStateEntry(stateFilePath string, t time.Time, r ReleaseInfo) error {
	data := StateEntry{CheckedForUpdateAt: t, LatestRelease: r}
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	ioutil.WriteFile(stateFilePath, content, 0600)

	return nil
}

func versionGreaterThan(v, w string) bool {
	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}
