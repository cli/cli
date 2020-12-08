package set

import (
	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/cmd/secret/shared"
	"github.com/cli/cli/pkg/prompt"
)

const (
	orgType  = "Organization"
	repoType = "Repository"
)

func promptSecretType() (secretType string, err error) {
	err = prompt.SurveyAskOne(&survey.Select{
		Message: "What type of secret would you like to set?",
		Options: []string{repoType, orgType},
	}, &secretType)

	return
}

func promptSecretName() (secretName string, err error) {
	err = prompt.SurveyAskOne(&survey.Input{
		Message: "Secret name:",
	}, &secretName)

	return
}

func promptSecretBody() (secretBody string, err error) {
	err = prompt.SurveyAskOne(&survey.Multiline{
		Message: "Paste your secret:",
	}, &secretBody)

	return
}

func promptVisibility() (visibility string, err error) {
	var choice int
	err = prompt.SurveyAskOne(&survey.Select{
		Message: "What visibility should this organization's repositories have for this secret?",
		Options: []string{
			"All organization repositories can use this secret",
			"Only private organization repositories can use this secret",
			"Only selected organization repositories can use this secret",
		},
	}, &choice)

	if err != nil {
		return
	}

	switch choice {
	case 0:
		visibility = shared.VisAll
	case 1:
		visibility = shared.VisPrivate
	case 2:
		visibility = shared.VisSelected
	}

	return
}

func promptRepositoryNames(client *api.Client) ([]string, error) {
	// TODO get all repositories for organization
	repositoryNames := []string{"TODO"}

	var selected []string

	err := prompt.SurveyAskOne(&survey.MultiSelect{
		Message: "Which repositories should be able to use this secret?",
		Options: repositoryNames,
	}, &selected)
	if err != nil {
		return nil, err
	}

	return selected, nil
}
