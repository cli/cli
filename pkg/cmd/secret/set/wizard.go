package set

import (
	"fmt"

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

func promptSecretBody() ([]byte, error) {
	var sb string
	err := prompt.SurveyAskOne(&survey.Multiline{
		Message: "Paste your secret:",
	}, &sb)
	if err != nil {
		return nil, err
	}

	return []byte(sb), nil
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

func promptRepositories(client *api.Client, host, orgName string) ([]api.OrgRepo, error) {
	orgRepos, err := api.OrganizationRepos(client, host, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to determine organization's repositories: %w", err)
	}

	options := []string{}
	for _, or := range orgRepos {
		options = append(options, or.Name)

	}

	var selectedNames []string

	err = prompt.SurveyAskOne(&survey.MultiSelect{
		Message: "Which repositories should be able to use this secret?",
		Options: options,
	}, &selectedNames)
	if err != nil {
		return nil, err
	}

	selected := []api.OrgRepo{}

	for _, name := range selectedNames {
		for _, or := range orgRepos {
			if name == or.Name {
				selected = append(selected, or)
				break
			}
		}
	}

	return selected, nil
}

func promptOrgName(defaultOrgName string) (orgName string, err error) {
	err = prompt.SurveyAskOne(&survey.Input{
		Message: "Organization name:",
		Default: defaultOrgName,
	}, &orgName)

	return
}
