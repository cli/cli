package command

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/pkg/surveyext"
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
	SubmitAction
	CancelAction
)

var SurveyAsk = func(qs []*survey.Question, response interface{}, opts ...survey.AskOpt) error {
	return survey.Ask(qs, response, opts...)
}

func confirmSubmission() (Action, error) {
	confirmAnswers := struct {
		Confirmation int
	}{}
	confirmQs := []*survey.Question{
		{
			Name: "confirmation",
			Prompt: &survey.Select{
				Message: "What's next?",
				Default: "Submit",
				Options: []string{
					"Preview in browser",
					"Submit",
					"Cancel",
				},
			},
		},
	}

	err := SurveyAsk(confirmQs, &confirmAnswers)
	if err != nil {
		return -1, fmt.Errorf("could not prompt: %w", err)
	}

	return Action(confirmAnswers.Confirmation), nil
}

func selectTemplate(templatePaths []string) (string, error) {
	templateResponse := struct {
		Index int
	}{}
	if len(templatePaths) > 1 {
		templateNames := make([]string, 0, len(templatePaths))
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
		if err := SurveyAsk(selectQs, &templateResponse); err != nil {
			return "", fmt.Errorf("could not prompt: %w", err)
		}
	}

	templateContents := githubtemplate.ExtractContents(templatePaths[templateResponse.Index])
	return string(templateContents), nil
}

func titleBodySurvey(cmd *cobra.Command, providedTitle, providedBody string, defs defaults, templatePaths []string) (*titleBody, error) {
	var inProgress titleBody
	inProgress.Title = defs.Title
	templateContents := ""

	if providedBody == "" {
		if len(templatePaths) > 0 {
			var err error
			templateContents, err = selectTemplate(templatePaths)
			if err != nil {
				return nil, err
			}
			inProgress.Body = templateContents
		} else {
			inProgress.Body = defs.Body
		}
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

	var qs []*survey.Question
	if providedTitle == "" {
		qs = append(qs, titleQuestion)
	}
	if providedBody == "" {
		qs = append(qs, bodyQuestion)
	}

	err := SurveyAsk(qs, &inProgress)
	if err != nil {
		return nil, fmt.Errorf("could not prompt: %w", err)
	}

	if inProgress.Body == "" {
		inProgress.Body = templateContents
	}

	wantsExtraFields := false
	prompt := &survey.Confirm{
		Message: "Would you like to add more information?",
	}
	err = survey.AskOne(prompt, &wantsExtraFields)
	if err != nil {
		return nil, fmt.Errorf("could not prompt: %w", err)
	}

	if wantsExtraFields {
		err = askExtraFields([]string{"Reviewers", "Assignees", "Labels", "Projects", "Milestone"})
		if err != nil {
			return nil, fmt.Errorf("could not prompt: %w", err)
		}
	}

	confirmA, err := confirmSubmission()
	if err != nil {
		return nil, fmt.Errorf("unable to confirm: %w", err)
	}

	inProgress.Action = confirmA

	return &inProgress, nil
}

func askExtraFields(extraFieldsSelect []string) error {
	enabled := func(q string) bool {
		for _, f := range extraFieldsSelect {
			if f == q {
				return true
			}
		}
		return false
	}

	if enabled("Reviewers") {
		var selectedReviewers []string
		reviewers := &survey.MultiSelect{
			Message: "Reviewers",
			Options: fakeUsersAndTeams,
		}
		err := survey.AskOne(reviewers, &selectedReviewers)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	}

	if enabled("Assignees") {
		var selectedAssignees []string
		assignees := &survey.MultiSelect{
			Message: "Assignees",
			Options: fakeUsersAndTeams,
		}
		err := survey.AskOne(assignees, &selectedAssignees)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	}

	if enabled("Labels") {
		var selectedLabels []string
		labels := &survey.MultiSelect{
			Message: "Labels",
			Options: []string{
				"bug",
				"discuss",
				"documentation",
				"feature",
				"help wanted",
			},
		}
		err := survey.AskOne(labels, &selectedLabels)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	}

	if enabled("Projects") {
		var selectedProjects []string
		projects := &survey.MultiSelect{
			Message: "Projects",
			Options: []string{
				"Roadmap",
				"Q4 Planning",
				"Q/A Testing",
			},
		}
		err := survey.AskOne(projects, &selectedProjects)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	}

	if enabled("Milestone") {
		var selectedMilestone string
		milestone := &survey.Select{
			Message: "Milestone",
			Options: []string{
				"(none)",
				"1.1-release",
				"1.2-beta",
			},
		}
		err := survey.AskOne(milestone, &selectedMilestone)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}
	}

	return nil
}
