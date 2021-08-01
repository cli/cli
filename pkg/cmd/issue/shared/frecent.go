package shared

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/prompt"
	"gopkg.in/yaml.v3"
)

func SelectFrecent() (string, error) {
	// TODO study other state access
	// TODO read from state file
	// TODO prompt
	frecent, err := getFrecentEntry(defaultFrecentPath())
	if err != nil {
		return "", err
	}
	choice := ""
	err = prompt.SurveyAskOne(&survey.Select{
		Message: "Which issue?",
		Options: frecent.Issues,
	}, &choice)
	if err != nil {
		return "", err
	}

	return choice, nil
}

type FrecentEntry struct {
	Issues []string
}

func defaultFrecentPath() string {
	return filepath.Join(config.StateDir(), "frecent.yml")
}

func getFrecentEntry(stateFilePath string) (*FrecentEntry, error) {
	content, err := ioutil.ReadFile(stateFilePath)
	if err != nil {
		return nil, err
	}

	var stateEntry FrecentEntry
	err = yaml.Unmarshal(content, &stateEntry)
	if err != nil {
		return nil, err
	}

	return &stateEntry, nil
}

/*
func setFrecentEntry(stateFilePath string, t time.Time, r ReleaseInfo) error {
	data := FrecentEntry{Issues: []string{"TODO"}}
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(stateFilePath), 0755)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(stateFilePath, content, 0600)
	return err
}

*/
