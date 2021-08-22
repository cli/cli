package shared

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/set"
	"github.com/cli/cli/pkg/surveyext"
)

type Editable struct {
	Title     EditableString
	Body      EditableString
	Base      EditableString
	Reviewers EditableSlice
	Assignees EditableSlice
	Labels    EditableSlice
	Projects  EditableSlice
	Milestone EditableString
	Metadata  api.RepoMetadataResult
}

type EditableString struct {
	Value   string
	Default string
	Options []string
	Edited  bool
}

type EditableSlice struct {
	Value   []string
	Add     []string
	Remove  []string
	Default []string
	Options []string
	Edited  bool
	Allowed bool
}

func (e Editable) Dirty() bool {
	return e.Title.Edited ||
		e.Body.Edited ||
		e.Base.Edited ||
		e.Reviewers.Edited ||
		e.Assignees.Edited ||
		e.Labels.Edited ||
		e.Projects.Edited ||
		e.Milestone.Edited
}

func (e Editable) TitleValue() *string {
	if !e.Title.Edited {
		return nil
	}
	return &e.Title.Value
}

func (e Editable) BodyValue() *string {
	if !e.Body.Edited {
		return nil
	}
	return &e.Body.Value
}

func (e Editable) ReviewerIds() (*[]string, *[]string, error) {
	if !e.Reviewers.Edited {
		return nil, nil, nil
	}
	if len(e.Reviewers.Add) != 0 || len(e.Reviewers.Remove) != 0 {
		s := set.NewStringSet()
		s.AddValues(e.Reviewers.Default)
		s.AddValues(e.Reviewers.Add)
		s.RemoveValues(e.Reviewers.Remove)
		e.Reviewers.Value = s.ToSlice()
	}
	var userReviewers []string
	var teamReviewers []string
	for _, r := range e.Reviewers.Value {
		if strings.ContainsRune(r, '/') {
			teamReviewers = append(teamReviewers, r)
		} else {
			userReviewers = append(userReviewers, r)
		}
	}
	userIds, err := e.Metadata.MembersToIDs(userReviewers)
	if err != nil {
		return nil, nil, err
	}
	teamIds, err := e.Metadata.TeamsToIDs(teamReviewers)
	if err != nil {
		return nil, nil, err
	}
	return &userIds, &teamIds, nil
}

func (e Editable) AssigneeIds(client *api.Client, repo ghrepo.Interface) (*[]string, error) {
	if !e.Assignees.Edited {
		return nil, nil
	}
	if len(e.Assignees.Add) != 0 || len(e.Assignees.Remove) != 0 {
		meReplacer := NewMeReplacer(client, repo.RepoHost())
		s := set.NewStringSet()
		s.AddValues(e.Assignees.Default)
		add, err := meReplacer.ReplaceSlice(e.Assignees.Add)
		if err != nil {
			return nil, err
		}
		s.AddValues(add)
		remove, err := meReplacer.ReplaceSlice(e.Assignees.Remove)
		if err != nil {
			return nil, err
		}
		s.RemoveValues(remove)
		e.Assignees.Value = s.ToSlice()
	}
	a, err := e.Metadata.MembersToIDs(e.Assignees.Value)
	return &a, err
}

func (e Editable) LabelIds() (*[]string, error) {
	if !e.Labels.Edited {
		return nil, nil
	}
	if len(e.Labels.Add) != 0 || len(e.Labels.Remove) != 0 {
		s := set.NewStringSet()
		s.AddValues(e.Labels.Default)
		s.AddValues(e.Labels.Add)
		s.RemoveValues(e.Labels.Remove)
		e.Labels.Value = s.ToSlice()
	}
	l, err := e.Metadata.LabelsToIDs(e.Labels.Value)
	return &l, err
}

func (e Editable) ProjectIds() (*[]string, error) {
	if !e.Projects.Edited {
		return nil, nil
	}
	if len(e.Projects.Add) != 0 || len(e.Projects.Remove) != 0 {
		s := set.NewStringSet()
		s.AddValues(e.Projects.Default)
		s.AddValues(e.Projects.Add)
		s.RemoveValues(e.Projects.Remove)
		e.Projects.Value = s.ToSlice()
	}
	p, err := e.Metadata.ProjectsToIDs(e.Projects.Value)
	return &p, err
}

func (e Editable) MilestoneId() (*string, error) {
	if !e.Milestone.Edited {
		return nil, nil
	}
	if e.Milestone.Value == noMilestone || e.Milestone.Value == "" {
		s := ""
		return &s, nil
	}
	m, err := e.Metadata.MilestoneToID(e.Milestone.Value)
	return &m, err
}

func EditFieldsSurvey(editable *Editable, editorCommand string) error {
	var err error
	if editable.Title.Edited {
		editable.Title.Value, err = titleSurvey(editable.Title.Default)
		if err != nil {
			return err
		}
	}
	if editable.Body.Edited {
		editable.Body.Value, err = bodySurvey(editable.Body.Default, editorCommand)
		if err != nil {
			return err
		}
	}
	if editable.Reviewers.Edited {
		editable.Reviewers.Value, err = multiSelectSurvey("Reviewers", editable.Reviewers.Default, editable.Reviewers.Options)
		if err != nil {
			return err
		}
	}
	if editable.Assignees.Edited {
		editable.Assignees.Value, err = multiSelectSurvey("Assignees", editable.Assignees.Default, editable.Assignees.Options)
		if err != nil {
			return err
		}
	}
	if editable.Labels.Edited {
		editable.Labels.Value, err = multiSelectSurvey("Labels", editable.Labels.Default, editable.Labels.Options)
		if err != nil {
			return err
		}
	}
	if editable.Projects.Edited {
		editable.Projects.Value, err = multiSelectSurvey("Projects", editable.Projects.Default, editable.Projects.Options)
		if err != nil {
			return err
		}
	}
	if editable.Milestone.Edited {
		editable.Milestone.Value, err = milestoneSurvey(editable.Milestone.Default, editable.Milestone.Options)
		if err != nil {
			return err
		}
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

func FieldsToEditSurvey(editable *Editable) error {
	contains := func(s []string, str string) bool {
		for _, v := range s {
			if v == str {
				return true
			}
		}
		return false
	}

	opts := []string{"Title", "Body"}
	if editable.Reviewers.Allowed {
		opts = append(opts, "Reviewers")
	}
	opts = append(opts, "Assignees", "Labels", "Projects", "Milestone")
	results, err := multiSelectSurvey("What would you like to edit?", []string{}, opts)
	if err != nil {
		return err
	}

	if contains(results, "Title") {
		editable.Title.Edited = true
	}
	if contains(results, "Body") {
		editable.Body.Edited = true
	}
	if contains(results, "Reviewers") {
		editable.Reviewers.Edited = true
	}
	if contains(results, "Assignees") {
		editable.Assignees.Edited = true
	}
	if contains(results, "Labels") {
		editable.Labels.Edited = true
	}
	if contains(results, "Projects") {
		editable.Projects.Edited = true
	}
	if contains(results, "Milestone") {
		editable.Milestone.Edited = true
	}

	return nil
}

func FetchOptions(client *api.Client, repo ghrepo.Interface, editable *Editable) error {
	input := api.RepoMetadataInput{
		Reviewers:  editable.Reviewers.Edited,
		Assignees:  editable.Assignees.Edited,
		Labels:     editable.Labels.Edited,
		Projects:   editable.Projects.Edited,
		Milestones: editable.Milestone.Edited,
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

	editable.Metadata = *metadata
	editable.Reviewers.Options = append(users, teams...)
	editable.Assignees.Options = users
	editable.Labels.Options = labels
	editable.Projects.Options = projects
	editable.Milestone.Options = milestones

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
		EditorCommand: editorCommand,
		Editor: &survey.Editor{
			Message:       "Body",
			FileName:      "*.md",
			Default:       body,
			HideDefault:   true,
			AppendDefault: true,
		},
	}
	err := survey.AskOne(q, &result)
	return result, err
}

func multiSelectSurvey(message string, defaults, options []string) ([]string, error) {
	if len(options) == 0 {
		return nil, nil
	}
	var results []string
	q := &survey.MultiSelect{
		Message: message,
		Options: options,
		Default: defaults,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func milestoneSurvey(title string, opts []string) (string, error) {
	if len(opts) == 0 {
		return "", nil
	}
	var result string
	q := &survey.Select{
		Message: "Milestone",
		Options: opts,
		Default: title,
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
