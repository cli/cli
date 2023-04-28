package iapi

import (
	"fmt"
	"sort"

	"github.com/AlecAivazis/survey/v2"
)

func queryTypeSurvey(opts []string) (string, error) {
	sort.Strings(opts)
	var result string
	q := &survey.Select{
		Message: "Object to query",
		Options: opts,
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func nextSurvey() (string, error) {
	var result string
	q := &survey.Select{
		Message: "What is next",
		Options: []string{"Add arguments", "Add fields", "Show query", "Run query", "Exit"},
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func objectsSurvey(opts []string) (string, error) {
	sort.Strings(opts)
	var result string
	q := &survey.Select{
		Message: "Object to modify",
		Options: opts,
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func fieldsSurvey(opts []string) ([]string, error) {
	if len(opts) == 0 {
		fmt.Println("No fields available")
		return nil, nil
	}
	sort.Strings(opts)
	var result []string
	q := &survey.MultiSelect{
		Message: "Fields to retrieve",
		Options: opts,
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func argumentsSurvey(opts []string) (map[string]string, error) {
	if len(opts) == 0 {
		fmt.Println("No arguments available")
		return nil, nil
	}
	r := make(map[string]string)
	var results []string
	arguments := &survey.MultiSelect{
		Message: "Arguments to specify",
		Options: opts,
	}
	err := survey.AskOne(arguments, &results)
	if err != nil {
		return nil, err
	}
	for _, v := range results {
		var result string
		msg := fmt.Sprintf("%s", v)
		argument := &survey.Input{
			Message: msg,
		}
		err = survey.AskOne(argument, &result)
		if err != nil {
			return nil, err
		}
		r[v] = result
	}
	return r, nil
}
