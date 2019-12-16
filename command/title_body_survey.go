package command

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

type titleBody struct {
	Body  string
	Title string
}

const (
	_confirmed   = iota
	_unconfirmed = iota
	_cancel      = iota
)

func confirm() (int, error) {
	confirmAnswers := struct {
		Confirmation int
	}{}
	confirmQs := []*survey.Question{
		{
			Name: "confirmation",
			Prompt: &survey.Select{
				Message: "Submit?",
				Options: []string{
					"Yes",
					"Edit",
					"Cancel",
				},
			},
		},
	}

	err := survey.Ask(confirmQs, &confirmAnswers)
	if err != nil {
		return -1, errors.Wrap(err, "could not prompt")
	}

	return confirmAnswers.Confirmation, nil
}

func titleBodySurvey(cmd *cobra.Command, providedTitle string, providedBody string) (*titleBody, error) {
	inProgress := titleBody{}

	confirmed := false
	editor := determineEditor()

	for !confirmed {
		titleQuestion := &survey.Question{
			Name: "title",
			Prompt: &survey.Input{
				Message: "Title",
				Default: inProgress.Title,
			},
		}
		bodyQuestion := &survey.Question{
			Name: "body",
			Prompt: &survey.Editor{
				Message:       fmt.Sprintf("Body (%s)", editor),
				FileName:      "*.md",
				Default:       inProgress.Body,
				AppendDefault: true,
				Editor:        editor,
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
		switch confirmA {
		case _confirmed:
			confirmed = true
		case _unconfirmed:
			continue
		case _cancel:
			fmt.Fprintln(cmd.ErrOrStderr(), "Discarding.")
			return nil, nil
		default:
			panic("reached unreachable case")
		}

	}

	return &inProgress, nil
}
