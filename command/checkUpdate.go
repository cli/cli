package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/github/gh-cli/utils"
	"github.com/mattn/go-isatty"
)

const nwo = "github/homebrew-gh"

func CheckForUpdate() error {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return nil
	}

	latestVersion, err := getLastestVersion()
	if err != nil {
		return err
	}

	updateAvailable := latestVersion != Version
	if updateAvailable {
		fmt.Printf(utils.Cyan(`
New version of gh is available! %s → %s
Changelog: https://github.com/%s/releases/tag/%[2]s
Run 'brew upgrade github/gh/gh' to update!`)+"\n\n", Version, latestVersion, nwo)
	}

	return nil
}

func getLastestVersion() (string, error) {
	url := fmt.Sprint("https://api.github.com/repos/%s/releases/latest", nwo)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var j struct {
		LatestVersion string `json:"tag_name"`
	}

	json.Unmarshal(data, &j)
	return j.LatestVersion, nil
}
