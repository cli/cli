package command

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/mattn/go-isatty"
)

func CheckForUpdate() error {
	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return nil
	}

	latestVersion, err := getLastestVersion()
	if err != nil {
		return err
	}

	fmt.Printf("🌭 %+v %+v\n", latestVersion, Version)

	return nil
}

func getLastestVersion() (string, error) {
	url := "https://api.github.com/repos/github/homebrew-gh/releases/latest"
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
