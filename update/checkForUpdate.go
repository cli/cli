package update

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/github/gh-cli/command"
	"github.com/github/gh-cli/utils"
	"golang.org/x/crypto/ssh/terminal"
)

const nwo = "github/homebrew-gh"

type releaseInfo struct {
	Version string `json:"tag_name"`
	URL     string `json:"html_url"`
}

func RunWhileCheckingForUpdate(isProduction bool, f func()) {
	if isProduction {
		f()
		return
	}

	newReleaseChan := make(chan *releaseInfo)
	go checkForUpdate(newReleaseChan)
	f()

	newRelease := <-newReleaseChan
	if newRelease != nil {
		fmt.Printf(utils.Cyan(`
A new version of gh is available! %s → %s
Changelog: %s
Run 'brew upgrade gh' to update!`)+"\n\n", command.Version, newRelease.Version, newRelease.URL)
	}
}

func checkForUpdate(newReleaseChan chan *releaseInfo) {
	// Ignore if this stdout is not a tty
	if !terminal.IsTerminal(int(os.Stdout.Fd())) {
		newReleaseChan <- nil
		return
	}

	latestRelease, err := getLatestRelease()
	if err != nil {
		newReleaseChan <- nil
		return
	}

	updateAvailable := latestRelease.Version != command.Version

	if updateAvailable {
		newReleaseChan <- latestRelease
	} else {
		newReleaseChan <- nil
	}
}

func getLatestRelease() (*releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", nwo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var r releaseInfo
	json.Unmarshal(data, &r)
	return &r, nil
}
