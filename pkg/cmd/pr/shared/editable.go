package shared

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/surveyext"
	"github.com/shurcooL/githubv4"
)

type Editable struct {
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

func (e Editable) Dirty() bool {
	return e.TitleEdited ||
		e.BodyEdited ||
		e.ReviewersEdited ||
		e.AssigneesEdited ||
		e.LabelsEdited ||
		e.ProjectsEdited ||
		e.MilestoneEdited
}

func (e Editable) TitleParam() *githubv4.String {
	if !e.TitleEdited {
		return nil
	}
	s := githubv4.String(e.Title)
	return &s
}

func (e Editable) BodyParam() *githubv4.String {
	if !e.BodyEdited {
		return nil
	}
	s := githubv4.String(e.Body)
	return &s
}

func (e Editable) ReviewersParams() (*[]githubv4.ID, *[]githubv4.ID, error) {
	if !e.ReviewersEdited {
		return nil, nil, nil
	}
	var userReviewers []string
	var teamReviewers []string
	for _, r := range e.Reviewers {
		if strings.ContainsRune(r, '/') {
			teamReviewers = append(teamReviewers, r)
		} else {
			userReviewers = append(userReviewers, r)
		}
	}
	userIds, err := toParams(userReviewers, e.Metadata.MembersToIDs)
	if err != nil {
		return nil, nil, err
	}
	teamIds, err := toParams(teamReviewers, e.Metadata.TeamsToIDs)
	if err != nil {
		return nil, nil, err
	}
	return userIds, teamIds, nil
}

func (e Editable) AssigneesParam(client *api.Client, repo ghrepo.Interface) (*[]githubv4.ID, error) {
	if !e.AssigneesEdited {
		return nil, nil
	}
	meReplacer := NewMeReplacer(client, repo.RepoHost())
	assignees, err := meReplacer.ReplaceSlice(e.Assignees)
	if err != nil {
		return nil, err
	}
	return toParams(assignees, e.Metadata.MembersToIDs)
}

func (e Editable) LabelsParam() (*[]githubv4.ID, error) {
	if !e.LabelsEdited {
		return nil, nil
	}
	return toParams(e.Labels, e.Metadata.LabelsToIDs)
}

func (e Editable) ProjectsParam() (*[]githubv4.ID, error) {
	if !e.ProjectsEdited {
		return nil, nil
	}
	return toParams(e.Projects, e.Metadata.ProjectsToIDs)
}

func (e Editable) MilestoneParam() (*githubv4.ID, error) {
	if !e.MilestoneEdited {
		return nil, nil
	}
	if e.Milestone == noMilestone || e.Milestone == "" {
		return githubv4.NewID(nil), nil
	}
	return toParam(e.Milestone, e.Metadata.MilestoneToID)
}

func EditFieldsSurvey(editable *Editable, editorCommand string) error {
	var err error
	if editable.TitleEdited {
		editable.Title, err = titleSurvey(editable.TitleDefault)
		if err != nil {
			return err
		}
	}
	if editable.BodyEdited {
		editable.Body, err = bodySurvey(editable.BodyDefault, editorCommand)
		if err != nil {
			return err
		}
	}
	if editable.ReviewersEdited {
		editable.Reviewers, err = reviewersSurvey(editable.ReviewersDefault, editable.ReviewersOptions)
		if err != nil {
			return err
		}
	}
	if editable.AssigneesEdited {
		editable.Assignees, err = assigneesSurvey(editable.AssigneesDefault, editable.AssigneesOptions)
		if err != nil {
			return err
		}
	}
	if editable.LabelsEdited {
		editable.Labels, err = labelsSurvey(editable.LabelsDefault, editable.LabelsOptions)
		if err != nil {
			return err
		}
	}
	if editable.ProjectsEdited {
		editable.Projects, err = projectsSurvey(editable.ProjectsDefault, editable.ProjectsOptions)
		if err != nil {
			return err
		}
	}
	if editable.MilestoneEdited {
		editable.Milestone, err = milestoneSurvey(editable.MilestoneDefault, editable.MilestoneOptions)
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

	results := []string{}
	opts := []string{"Title", "Body"}
	if editable.ReviewersAllowed {
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
		editable.TitleEdited = true
	}
	if contains(results, "Body") {
		editable.BodyEdited = true
	}
	if contains(results, "Reviewers") {
		editable.ReviewersEdited = true
	}
	if contains(results, "Assignees") {
		editable.AssigneesEdited = true
	}
	if contains(results, "Labels") {
		editable.LabelsEdited = true
	}
	if contains(results, "Projects") {
		editable.ProjectsEdited = true
	}
	if contains(results, "Milestone") {
		editable.MilestoneEdited = true
	}

	return nil
}

func FetchOptions(client *api.Client, repo ghrepo.Interface, editable *Editable) error {
	input := api.RepoMetadataInput{
		Reviewers:  editable.ReviewersEdited,
		Assignees:  editable.AssigneesEdited,
		Labels:     editable.LabelsEdited,
		Projects:   editable.ProjectsEdited,
		Milestones: editable.MilestoneEdited,
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
	editable.ReviewersOptions = append(users, teams...)
	editable.AssigneesOptions = users
	editable.LabelsOptions = labels
	editable.ProjectsOptions = projects
	editable.MilestoneOptions = milestones

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

func reviewersSurvey(reviewers api.ReviewRequests, opts []string) ([]string, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	logins := []string{}
	for _, a := range reviewers.Nodes {
		logins = append(logins, a.RequestedReviewer.Login)
	}
	var results []string
	q := &survey.MultiSelect{
		Message: "Reviewers",
		Options: opts,
		Default: logins,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func assigneesSurvey(assignees api.Assignees, opts []string) ([]string, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	logins := []string{}
	for _, a := range assignees.Nodes {
		logins = append(logins, a.Login)
	}
	var results []string
	q := &survey.MultiSelect{
		Message: "Assignees",
		Options: opts,
		Default: logins,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func labelsSurvey(labels api.Labels, opts []string) ([]string, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	names := []string{}
	for _, l := range labels.Nodes {
		names = append(names, l.Name)
	}
	var results []string
	q := &survey.MultiSelect{
		Message: "Labels",
		Options: opts,
		Default: names,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func projectsSurvey(projectCards api.ProjectCards, opts []string) ([]string, error) {
	if len(opts) == 0 {
		return nil, nil
	}
	names := []string{}
	for _, c := range projectCards.Nodes {
		names = append(names, c.Project.Name)
	}
	var results []string
	q := &survey.MultiSelect{
		Message: "Projects",
		Options: opts,
		Default: names,
	}
	err := survey.AskOne(q, &results)
	return results, err
}

func milestoneSurvey(milestone api.Milestone, opts []string) (string, error) {
	if len(opts) == 0 {
		return "", nil
	}
	var result string
	q := &survey.Select{
		Message: "Milestone",
		Options: opts,
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

func toParams(s []string, mapper func([]string) ([]string, error)) (*[]githubv4.ID, error) {
	ids, err := mapper(s)
	if err != nil {
		return nil, err
	}
	gIds := make([]githubv4.ID, len(ids))
	for i, v := range ids {
		gIds[i] = v
	}
	return &gIds, nil
}

func toParam(s string, mapper func(string) (string, error)) (*githubv4.ID, error) {
	id, err := mapper(s)
	if err != nil {
		return nil, err
	}
	gId := githubv4.ID(id)
	return &gId, nil
}
