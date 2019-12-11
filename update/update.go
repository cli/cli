package update

import (
	"fmt"
	"io/ioutil"
	"time"

	"github.com/github/gh-cli/api"
	"github.com/hashicorp/go-version"
	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v3"
)

// ReleaseInfo stores information about a release
type ReleaseInfo struct {
	Version string
	URL     string
}

type StateEntry struct {
	CheckedForUpdateAt time.Time   `yaml:"checked_for_update_at"`
	LatestRelease      ReleaseInfo `yaml:"latest_release"`
}

// CheckForUpdate checks whether this software has had a newer relase on GitHub
func CheckForUpdate(client *api.Client, repo, currentVersion string) (*ReleaseInfo, error) {
	latestRelease, err := getLatestReleaseInfo(client, repo, currentVersion)
	if err != nil {
		return nil, err
	}

	if versionGreaterThan(latestRelease.Version, currentVersion) {
		return latestRelease, nil
	}

	return nil, nil
}

func getLatestReleaseInfo(client *api.Client, repo, currentVersion string) (*ReleaseInfo, error) {
	stateEntry, err := getStateEntry()
	if err != nil {
		return nil, err
	}

	checkedRecently := time.Since(stateEntry.CheckedForUpdateAt).Hours() < 24
	if checkedRecently {
		return &stateEntry.LatestRelease, nil
	}

	latestRelease := ReleaseInfo{}
	err = client.REST("GET", fmt.Sprintf("repos/%s/releases/latest", repo), nil, &latestRelease)
	if err != nil {
		return nil, err
	}

	err = setStateEntry(time.Now(), latestRelease)
	if err != nil {
		return nil, err
	}

	return &latestRelease, nil
}

func stateFileName() string {
	f, _ := homedir.Expand("~/.config/gh/state.yml")
	return f
}

func getStateEntry() (*StateEntry, error) {
	f := stateFileName()
	content, err := ioutil.ReadFile(f)
	if err != nil {
		data := StateEntry{
			CheckedForUpdateAt: time.Now(),
			LatestRelease: ReleaseInfo{
				Version: "v0.0.0",
				URL:     "<?>",
			},
		}
		content, err = yaml.Marshal(data)
		if err != nil {
			return nil, err
		}
		ioutil.WriteFile(f, content, 0600)
	}

	var stateEntry StateEntry
	err = yaml.Unmarshal(content, &stateEntry)
	if err != nil {
		return nil, err
	}

	return &stateEntry, nil
}

func setStateEntry(t time.Time, r ReleaseInfo) error {
	data := StateEntry{
		CheckedForUpdateAt: t,
		LatestRelease:      r,
	}
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	ioutil.WriteFile(stateFileName(), content, 0600)

	return nil
}

func versionGreaterThan(v, w string) bool {
	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}
