package shared

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/surveyext"
)

type EditableOptions struct {
	Title        string
	TitleDefault string
	TitleEdited  bool

	Body        string
	BodyDefault string
	BodyEdited  bool

	Reviewers        []string
	ReviewersDefault api.ReviewRequests
	ReviewersOptions []string
	ReviewersEdited  bool
	ReviewersAllowed bool

	Assignees        []string
	AssigneesDefault api.Assignees
	AssigneesOptions []string
	AssigneesEdited  bool

	Labels        []string
	LabelsDefault api.Labels
	LabelsOptions []string
	LabelsEdited  bool

	Projects        []string
	ProjectsDefault api.ProjectCards
	ProjectsOptions []string
	ProjectsEdited  bool

	Milestone        string
	MilestoneDefault api.Milestone
	MilestoneOptions []string
	MilestoneEdited  bool

	Metadata api.RepoMetadataResult
}

func (e EditableOptions) Dirty() bool {
	return e.TitleEdited ||
		e.BodyEdited ||
		e.ReviewersEdited ||
		e.AssigneesEdited ||
		e.LabelsEdited ||
		e.ProjectsEdited ||
		e.MilestoneEdited
}

func EditableSurvey(editorCommand string, options *EditableOptions) error {
	if options.TitleEdited {
		title, err := titleSurvey(options.TitleDefault)
		if err != nil {
			return err
		}
		options.Title = title
	}
	if options.BodyEdited {
		body, err := bodySurvey(options.BodyDefault, editorCommand)
		if err != nil {
			return err
		}
		options.Body = body
	}
	if options.AssigneesEdited {
		assignees, err := assigneesSurvey(options.AssigneesDefault, options.AssigneesOptions)
		if err != nil {
			return err
		}
		options.Assignees = assignees
	}
	if options.LabelsEdited {
		labels, err := labelsSurvey(options.LabelsDefault, options.LabelsOptions)
		if err != nil {
			return err
		}
		options.Labels = labels
	}
	if options.ProjectsEdited {
		projects, err := projectsSurvey(options.ProjectsDefault, options.ProjectsOptions)
		if err != nil {
			return err
		}
		options.Projects = projects
	}
	if options.MilestoneEdited {
		milestone, err := milestoneSurvey(options.MilestoneDefault, options.MilestoneOptions)
		if err != nil {
			return err
		}
		options.Milestone = milestone
	}
	confirm, err := confirmSurvey()
	if err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("Discarding...")
	}

	return nil
}

func FieldsToEditSurvey(options *EditableOptions) error {
	contains := func(s []string, str string) bool {
		for _, v := range s {
			if v == str {
				return true
			}
		}
		return false
	}

	results := []string{}
	opts := []string{"Title", "Body"}
	if options.ReviewersAllowed {
		opts = append(opts, "Reviewers")
	}
	opts = append(opts, "Assignees", "Labels", "Projects", "Milestone")
	q := &survey.MultiSelect{
		Message: "What would you like to edit?",
		Options: opts,
	}
	err := survey.AskOne(q, &results)
	if err != nil {
		return err
	}

	if contains(results, "Title") {
		options.TitleEdited = true
	}
	if contains(results, "Body") {
		options.BodyEdited = true
	}
	if contains(results, "Reviewers") {
		options.ReviewersEdited = true
	}
	if contains(results, "Assignees") {
		options.AssigneesEdited = true
	}
	if contains(results, "Labels") {
		options.LabelsEdited = true
	}
	if contains(results, "Projects") {
		options.ProjectsEdited = true
	}
	if contains(results, "Milestone") {
		options.MilestoneEdited = true
	}

	return nil
}

func FetchOptions(client *api.Client, repo ghrepo.Interface, options *EditableOptions) error {
	input := api.RepoMetadataInput{
		Reviewers:  options.ReviewersEdited,
		Assignees:  options.AssigneesEdited,
		Labels:     options.LabelsEdited,
		Projects:   options.ProjectsEdited,
		Milestones: options.MilestoneEdited,
	}
	metadata, err := api.RepoMetadata(client, repo, input)
	if err != nil {
		return err
	}

	var users []string
	for _, u := range metadata.AssignableUsers {
		users = append(users, u.Login)
	}
	var teams []string
	for _, t := range metadata.Teams {
		teams = append(teams, fmt.Sprintf("%s/%s", repo.RepoOwner(), t.Slug))
	}
	var labels []string
	for _, l := range metadata.Labels {
		labels = append(labels, l.Name)
	}
	var projects []string
	for _, l := range metadata.Projects {
		projects = append(projects, l.Name)
	}
	milestones := []string{noMilestone}
	for _, m := range metadata.Milestones {
		milestones = append(milestones, m.Title)
	}

	options.Metadata = *metadata
	options.ReviewersOptions = append(users, teams...)
	options.AssigneesOptions = users
	options.LabelsOptions = labels
	options.ProjectsOptions = projects
	options.MilestoneOptions = milestones

	return nil
}

func titleSurvey(title string) (string, error) {
	var result string
	q := &survey.Input{
		Message: "Title",
		Default: title,
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func bodySurvey(body, editorCommand string) (string, error) {
	var result string
	q := &surveyext.GhEditor{
		BlankAllowed:  true,
		EditorCommand: editorCommand,
		Editor: &survey.Editor{Message: "Body",
			FileName:      "*.md",
			Default:       body,
			HideDefault:   true,
			AppendDefault: true,
		},
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func assigneesSurvey(assignees api.Assignees, assigneesOpts []string) ([]string, error) {
	if len(assigneesOpts) == 0 {
		return nil, nil
	}
	logins := []string{}
	for _, a := range assignees.Nodes {
		logins = append(logins, a.Login)
	}
	var results []string
	q := &survey.MultiSelect{
		Message: "Assignees",
		Options: assigneesOpts,
		Default: logins,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func labelsSurvey(labels api.Labels, labelOpts []string) ([]string, error) {
	if len(labelOpts) == 0 {
		return nil, nil
	}
	names := []string{}
	for _, l := range labels.Nodes {
		names = append(names, l.Name)
	}
	var results []string
	q := &survey.MultiSelect{
		Message: "Labels",
		Options: labelOpts,
		Default: names,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func projectsSurvey(projectCards api.ProjectCards, projectsOpts []string) ([]string, error) {
	if len(projectsOpts) == 0 {
		return nil, nil
	}
	names := []string{}
	for _, c := range projectCards.Nodes {
		names = append(names, c.Project.Name)
	}
	var results []string
	q := &survey.MultiSelect{
		Message: "Projects",
		Options: projectsOpts,
		Default: names,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func milestoneSurvey(milestone api.Milestone, milestoneOpts []string) (string, error) {
	if len(milestoneOpts) == 0 {
		return "", nil
	}
	var result string
	q := &survey.Select{
		Message: "Milestone",
		Options: milestoneOpts,
		Default: milestone.Title,
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func confirmSurvey() (bool, error) {
	var result bool
	q := &survey.Confirm{
		Message: "Submit?",
		Default: true,
	}
	err := survey.AskOne(q, &result)
	return result, err
}
