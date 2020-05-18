package command

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type Action int

type issueMetadataState struct {
	Body   string
	Title  string
	Action Action

	Metadata   []string
	Reviewers  []string
	Assignees  []string
	Labels     []string
	Projects   []string
	Milestones []string

	MetadataResult *api.RepoMetadataResult
}

func (tb *issueMetadataState) HasMetadata() bool {
	return len(tb.Reviewers) > 0 ||
		len(tb.Assignees) > 0 ||
		len(tb.Labels) > 0 ||
		len(tb.Projects) > 0 ||
		len(tb.Milestones) > 0
}

const (
	SubmitAction Action = iota
	PreviewAction
	CancelAction
	MetadataAction

	noMilestone = "(none)"
)

var SurveyAsk = func(qs []*survey.Question, response interface{}, opts ...survey.AskOpt) error {
	return survey.Ask(qs, response, opts...)
}

func confirmSubmission(allowPreview bool, allowMetadata bool) (Action, error) {
	const (
		submitLabel   = "Submit"
		previewLabel  = "Continue in browser"
		metadataLabel = "Add metadata"
		cancelLabel   = "Cancel"
	)

	options := []string{submitLabel}
	if allowPreview {
		options = append(options, previewLabel)
	}
	if allowMetadata {
		options = append(options, metadataLabel)
	}
	options = append(options, cancelLabel)

	confirmAnswers := struct {
		Confirmation int
	}{}
	confirmQs := []*survey.Question{
		{
			Name: "confirmation",
			Prompt: &survey.Select{
				Message: "What's next?",
				Options: options,
			},
		},
	}

	err := SurveyAsk(confirmQs, &confirmAnswers)
	if err != nil {
		return -1, fmt.Errorf("could not prompt: %w", err)
	}

	switch options[confirmAnswers.Confirmation] {
	case submitLabel:
		return SubmitAction, nil
	case previewLabel:
		return PreviewAction, nil
	case metadataLabel:
		return MetadataAction, nil
	case cancelLabel:
		return CancelAction, nil
	default:
		return -1, fmt.Errorf("invalid index: %d", confirmAnswers.Confirmation)
	}
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

func titleBodySurvey(cmd *cobra.Command, issueState *issueMetadataState, apiClient *api.Client, repo ghrepo.Interface, providedTitle, providedBody string, defs defaults, templatePaths []string, allowReviewers, allowMetadata bool) error {
	editorCommand, err := determineEditor(cmd)
	if err != nil {
		return err
	}

	issueState.Title = defs.Title
	templateContents := ""

	if providedBody == "" {
		if len(templatePaths) > 0 {
			var err error
			templateContents, err = selectTemplate(templatePaths)
			if err != nil {
				return err
			}
			issueState.Body = templateContents
		} else {
			issueState.Body = defs.Body
		}
	}

	titleQuestion := &survey.Question{
		Name: "title",
		Prompt: &survey.Input{
			Message: "Title",
			Default: issueState.Title,
		},
	}
	bodyQuestion := &survey.Question{
		Name: "body",
		Prompt: &surveyext.GhEditor{
			BlankAllowed:  true,
			EditorCommand: editorCommand,
			Editor: &survey.Editor{
				Message:       "Body",
				FileName:      "*.md",
				Default:       issueState.Body,
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

	err = SurveyAsk(qs, issueState)
	if err != nil {
		return fmt.Errorf("could not prompt: %w", err)
	}

	if issueState.Body == "" {
		issueState.Body = templateContents
	}

	allowPreview := !issueState.HasMetadata()
	confirmA, err := confirmSubmission(allowPreview, allowMetadata)
	if err != nil {
		return fmt.Errorf("unable to confirm: %w", err)
	}

	if confirmA == MetadataAction {
		isChosen := func(m string) bool {
			for _, c := range issueState.Metadata {
				if m == c {
					return true
				}
			}
			return false
		}

		extraFieldsOptions := []string{}
		if allowReviewers {
			extraFieldsOptions = append(extraFieldsOptions, "Reviewers")
		}
		extraFieldsOptions = append(extraFieldsOptions, "Assignees", "Labels", "Projects", "Milestone")

		err = SurveyAsk([]*survey.Question{
			{
				Name: "metadata",
				Prompt: &survey.MultiSelect{
					Message: "What would you like to add?",
					Options: extraFieldsOptions,
				},
			},
		}, issueState)
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		metadataInput := api.RepoMetadataInput{
			Reviewers:  isChosen("Reviewers"),
			Assignees:  isChosen("Assignees"),
			Labels:     isChosen("Labels"),
			Projects:   isChosen("Projects"),
			Milestones: isChosen("Milestone"),
		}
		s := utils.Spinner(cmd.OutOrStderr())
		utils.StartSpinner(s)
		issueState.MetadataResult, err = api.RepoMetadata(apiClient, repo, metadataInput)
		utils.StopSpinner(s)
		if err != nil {
			return fmt.Errorf("error fetching metadata options: %w", err)
		}

		var users []string
		for _, u := range issueState.MetadataResult.AssignableUsers {
			users = append(users, u.Login)
		}
		var teams []string
		for _, t := range issueState.MetadataResult.Teams {
			teams = append(teams, fmt.Sprintf("%s/%s", repo.RepoOwner(), t.Slug))
		}
		var labels []string
		for _, l := range issueState.MetadataResult.Labels {
			labels = append(labels, l.Name)
		}
		var projects []string
		for _, l := range issueState.MetadataResult.Projects {
			projects = append(projects, l.Name)
		}
		milestones := []string{noMilestone}
		for _, m := range issueState.MetadataResult.Milestones {
			milestones = append(milestones, m.Title)
		}

		var mqs []*survey.Question
		if isChosen("Reviewers") {
			if len(users) > 0 || len(teams) > 0 {
				mqs = append(mqs, &survey.Question{
					Name: "reviewers",
					Prompt: &survey.MultiSelect{
						Message: "Reviewers",
						Options: append(users, teams...),
						Default: issueState.Reviewers,
					},
				})
			} else {
				cmd.PrintErrln("warning: no available reviewers")
			}
		}
		if isChosen("Assignees") {
			if len(users) > 0 {
				mqs = append(mqs, &survey.Question{
					Name: "assignees",
					Prompt: &survey.MultiSelect{
						Message: "Assignees",
						Options: users,
						Default: issueState.Assignees,
					},
				})
			} else {
				cmd.PrintErrln("warning: no assignable users")
			}
		}
		if isChosen("Labels") {
			if len(labels) > 0 {
				mqs = append(mqs, &survey.Question{
					Name: "labels",
					Prompt: &survey.MultiSelect{
						Message: "Labels",
						Options: labels,
						Default: issueState.Labels,
					},
				})
			} else {
				cmd.PrintErrln("warning: no labels in the repository")
			}
		}
		if isChosen("Projects") {
			if len(projects) > 0 {
				mqs = append(mqs, &survey.Question{
					Name: "projects",
					Prompt: &survey.MultiSelect{
						Message: "Projects",
						Options: projects,
						Default: issueState.Projects,
					},
				})
			} else {
				cmd.PrintErrln("warning: no projects to choose from")
			}
		}
		if isChosen("Milestone") {
			if len(milestones) > 1 {
				var milestoneDefault interface{}
				if len(issueState.Milestones) > 0 {
					milestoneDefault = issueState.Milestones[0]
				}
				mqs = append(mqs, &survey.Question{
					Name: "milestone",
					Prompt: &survey.Select{
						Message: "Milestone",
						Options: milestones,
						Default: milestoneDefault,
					},
				})
			} else {
				cmd.PrintErrln("warning: no milestones in the repository")
			}
		}

		err = SurveyAsk(mqs, issueState, survey.WithKeepFilter(true))
		if err != nil {
			return fmt.Errorf("could not prompt: %w", err)
		}

		if len(issueState.Milestones) > 0 && issueState.Milestones[0] == noMilestone {
			issueState.Milestones = issueState.Milestones[0:0]
		}

		allowPreview = !issueState.HasMetadata()
		allowMetadata = false
		confirmA, err = confirmSubmission(allowPreview, allowMetadata)
		if err != nil {
			return fmt.Errorf("unable to confirm: %w", err)
		}
	}

	issueState.Action = confirmA
	return nil
}
