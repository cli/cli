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

func CheckForUpdate(handleUpdate chan func()) {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		handleUpdate <- nil
	}

	latestVersion, err := getLastestVersion()
	if err != nil {
		handleUpdate <- nil
	}

	updateAvailable := latestVersion != Version

	if updateAvailable {
		handleUpdate <- func() {
			fmt.Printf(utils.Cyan(`
A new version of gh is available! %s → %s
Changelog: https://github.com/%s/releases/tag/%[2]s
Run 'brew reinstall gh' to update!`)+"\n\n", Version, latestVersion, nwo)
		}
	} else {
		handleUpdate <- nil
	}
}

func getLastestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", nwo)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsedData struct {
		LatestVersion string `json:"tag_name"`
	}

	json.Unmarshal(data, &parsedData)
	return parsedData.LatestVersion, nil
}
