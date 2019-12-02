package update

import (
	"fmt"
	"os"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/command"
	"github.com/github/gh-cli/utils"
)

const nwo = "github/homebrew-gh"

type releaseInfo struct {
	Version string `json:"tag_name"`
	URL     string `json:"html_url"`
}

func UpdateMessage(client *api.Client) *string {
	latestRelease, err := getLatestRelease(client)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%+v", err)
		return nil
	}

	updateAvailable := latestRelease.Version != command.Version
	if updateAvailable {
		alertMsg := fmt.Sprintf(utils.Cyan(`
		A new version of gh is available! %s → %s
		Changelog: %s
		Run 'brew upgrade gh' to update!`)+"\n\n", command.Version, latestRelease.Version, latestRelease.URL)
		return &alertMsg
	}

	return nil
}

func getLatestRelease(client *api.Client) (*releaseInfo, error) {
	path := fmt.Sprintf("repos/%s/releases/latest", nwo)
	var r releaseInfo
	err := client.REST("GET", path, &r)
	return &r, err
}
