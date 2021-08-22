package shared

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/githubtemplate"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/surveyext"
)

type Action int

const (
	SubmitAction Action = iota
	PreviewAction
	CancelAction
	MetadataAction
	EditCommitMessageAction

	noMilestone = "(none)"
)

func ConfirmSubmission(allowPreview bool, allowMetadata bool) (Action, error) {
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

	err := prompt.SurveyAsk(confirmQs, &confirmAnswers)
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

func BodySurvey(state *IssueMetadataState, templateContent, editorCommand string) error {
	if templateContent != "" {
		if state.Body != "" {
			// prevent excessive newlines between default body and template
			state.Body = strings.TrimRight(state.Body, "\n")
			state.Body += "\n\n"
		}
		state.Body += templateContent
	}

	preBody := state.Body

	// TODO should just be an AskOne but ran into problems with the stubber
	qs := []*survey.Question{
		{
			Name: "Body",
			Prompt: &surveyext.GhEditor{
				BlankAllowed:  true,
				EditorCommand: editorCommand,
				Editor: &survey.Editor{
					Message:       "Body",
					FileName:      "*.md",
					Default:       state.Body,
					HideDefault:   true,
					AppendDefault: true,
				},
			},
		},
	}

	err := prompt.SurveyAsk(qs, state)
	if err != nil {
		return err
	}

	if preBody != state.Body {
		state.MarkDirty()
	}

	return nil
}

func TitleSurvey(state *IssueMetadataState) error {
	preTitle := state.Title

	// TODO should just be an AskOne but ran into problems with the stubber
	qs := []*survey.Question{
		{
			Name: "Title",
			Prompt: &survey.Input{
				Message: "Title",
				Default: state.Title,
			},
		},
	}

	err := prompt.SurveyAsk(qs, state)
	if err != nil {
		return err
	}

	if preTitle != state.Title {
		state.MarkDirty()
	}

	return nil
}

type MetadataFetcher struct {
	IO        *iostreams.IOStreams
	APIClient *api.Client
	Repo      ghrepo.Interface
	State     *IssueMetadataState
}

func (mf *MetadataFetcher) RepoMetadataFetch(input api.RepoMetadataInput) (*api.RepoMetadataResult, error) {
	mf.IO.StartProgressIndicator()
	metadataResult, err := api.RepoMetadata(mf.APIClient, mf.Repo, input)
	mf.IO.StopProgressIndicator()
	mf.State.MetadataResult = metadataResult
	return metadataResult, err
}

type RepoMetadataFetcher interface {
	RepoMetadataFetch(api.RepoMetadataInput) (*api.RepoMetadataResult, error)
}

func MetadataSurvey(io *iostreams.IOStreams, baseRepo ghrepo.Interface, fetcher RepoMetadataFetcher, state *IssueMetadataState) error {
	isChosen := func(m string) bool {
		for _, c := range state.Metadata {
			if m == c {
				return true
			}
		}
		return false
	}

	allowReviewers := state.Type == PRMetadata

	extraFieldsOptions := []string{}
	if allowReviewers {
		extraFieldsOptions = append(extraFieldsOptions, "Reviewers")
	}
	extraFieldsOptions = append(extraFieldsOptions, "Assignees", "Labels", "Projects", "Milestone")

	err := prompt.SurveyAsk([]*survey.Question{
		{
			Name: "metadata",
			Prompt: &survey.MultiSelect{
				Message: "What would you like to add?",
				Options: extraFieldsOptions,
			},
		},
	}, state)
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
	metadataResult, err := fetcher.RepoMetadataFetch(metadataInput)
	if err != nil {
		return fmt.Errorf("error fetching metadata options: %w", err)
	}

	var users []string
	for _, u := range metadataResult.AssignableUsers {
		users = append(users, u.Login)
	}
	var teams []string
	for _, t := range metadataResult.Teams {
		teams = append(teams, fmt.Sprintf("%s/%s", baseRepo.RepoOwner(), t.Slug))
	}
	var labels []string
	for _, l := range metadataResult.Labels {
		labels = append(labels, l.Name)
	}
	var projects []string
	for _, l := range metadataResult.Projects {
		projects = append(projects, l.Name)
	}
	milestones := []string{noMilestone}
	for _, m := range metadataResult.Milestones {
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
					Default: state.Reviewers,
				},
			})
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no available reviewers")
		}
	}
	if isChosen("Assignees") {
		if len(users) > 0 {
			mqs = append(mqs, &survey.Question{
				Name: "assignees",
				Prompt: &survey.MultiSelect{
					Message: "Assignees",
					Options: users,
					Default: state.Assignees,
				},
			})
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no assignable users")
		}
	}
	if isChosen("Labels") {
		if len(labels) > 0 {
			mqs = append(mqs, &survey.Question{
				Name: "labels",
				Prompt: &survey.MultiSelect{
					Message: "Labels",
					Options: labels,
					Default: state.Labels,
				},
			})
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no labels in the repository")
		}
	}
	if isChosen("Projects") {
		if len(projects) > 0 {
			mqs = append(mqs, &survey.Question{
				Name: "projects",
				Prompt: &survey.MultiSelect{
					Message: "Projects",
					Options: projects,
					Default: state.Projects,
				},
			})
		} else {
			fmt.Fprintln(io.ErrOut, "warning: no projects to choose from")
		}
	}
	if isChosen("Milestone") {
		if len(milestones) > 1 {
			var milestoneDefault string
			if len(state.Milestones) > 0 {
				milestoneDefault = state.Milestones[0]
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
			fmt.Fprintln(io.ErrOut, "warning: no milestones in the repository")
		}
	}

	values := struct {
		Reviewers []string
		Assignees []string
		Labels    []string
		Projects  []string
		Milestone string
	}{}

	err = prompt.SurveyAsk(mqs, &values, survey.WithKeepFilter(true))
	if err != nil {
		return fmt.Errorf("could not prompt: %w", err)
	}

	if isChosen("Reviewers") {
		state.Reviewers = values.Reviewers
	}
	if isChosen("Assignees") {
		state.Assignees = values.Assignees
	}
	if isChosen("Labels") {
		state.Labels = values.Labels
	}
	if isChosen("Projects") {
		state.Projects = values.Projects
	}
	if isChosen("Milestone") {
		if values.Milestone != "" && values.Milestone != noMilestone {
			state.Milestones = []string{values.Milestone}
		} else {
			state.Milestones = []string{}
		}
	}

	return nil
}

func FindTemplates(dir, path string) ([]string, string) {
	if dir == "" {
		rootDir, err := git.ToplevelDir()
		if err != nil {
			return []string{}, ""
		}
		dir = rootDir
	}

	templateFiles := githubtemplate.FindNonLegacy(dir, path)
	legacyTemplate := githubtemplate.FindLegacy(dir, path)

	return templateFiles, legacyTemplate
}
