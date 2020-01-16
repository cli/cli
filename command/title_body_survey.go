package command

import (
	"github.com/AlecAivazis/survey/v2"
	"github.com/github/gh-cli/pkg/githubtemplate"
	"github.com/github/gh-cli/pkg/surveyext"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type Action int

type titleBody struct {
	Body   string
	Title  string
	Action Action
}

const (
	PreviewAction Action = iota
	SubmitAction  Action = iota
	CancelAction  Action = iota
)

func confirm() (Action, error) {
	confirmAnswers := struct {
		Confirmation int
	}{}
	confirmQs := []*survey.Question{
		{
			Name: "confirmation",
			Prompt: &survey.Select{
				Message: "What's next?",
				Options: []string{
					"Preview in browser",
					"Submit",
					"Cancel",
				},
			},
		},
	}

	err := survey.Ask(confirmQs, &confirmAnswers)
	if err != nil {
		return -1, errors.Wrap(err, "could not prompt")
	}

	return Action(confirmAnswers.Confirmation), nil
}

func selectTemplate(templatePaths []string) (string, error) {
	templateResponse := struct {
		Index int
	}{}
	if len(templatePaths) > 1 {
		templateNames := []string{}
		for _, p := range templatePaths {
			templateNames = append(templateNames, githubtemplate.ExtractName(p))
		}

		selectQs := []*survey.Question{
			{
				Name: "index",
				Prompt: &survey.Select{
					Message: "Choose a template",
					Options: templateNames,
				},
			},
		}
		if err := survey.Ask(selectQs, &templateResponse); err != nil {
			return "", errors.Wrap(err, "could not prompt")
		}
	}

	templateContents := githubtemplate.ExtractContents(templatePaths[templateResponse.Index])
	return string(templateContents), nil
}

func titleBodySurvey(cmd *cobra.Command, providedTitle string, providedBody string, templatePaths []string) (*titleBody, error) {
	inProgress := titleBody{}

	if providedBody == "" && len(templatePaths) > 0 {
		templateContents, err := selectTemplate(templatePaths)
		if err != nil {
			return nil, err
		}
		inProgress.Body = templateContents
	}

	titleQuestion := &survey.Question{
		Name: "title",
		Prompt: &survey.Input{
			Message: "Title",
			Default: inProgress.Title,
		},
	}
	bodyQuestion := &survey.Question{
		Name: "body",
		Prompt: &surveyext.GhEditor{
			Editor: &survey.Editor{
				Message:       "Body",
				FileName:      "*.md",
				Default:       inProgress.Body,
				HideDefault:   true,
				AppendDefault: true,
			},
		},
	}

	qs := []*survey.Question{}
	if providedTitle == "" {
		qs = append(qs, titleQuestion)
	}
	if providedBody == "" {
		qs = append(qs, bodyQuestion)
	}

	err := survey.Ask(qs, &inProgress)
	if err != nil {
		return nil, errors.Wrap(err, "could not prompt")
	}

	confirmA, err := confirm()
	if err != nil {
		return nil, errors.Wrap(err, "unable to confirm")
	}

	inProgress.Action = confirmA

	return &inProgress, nil
}
