package shared

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/set"
)

type Editable struct {
	Title     EditableString
	Body      EditableString
	Base      EditableString
	Reviewers EditableSlice
	Assignees EditableSlice
	Labels    EditableSlice
	Projects  EditableProjects
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

// ProjectsV2 mutations require a mapping of an item ID to a project ID.
// Keep that map along with standard EditableSlice data.
type EditableProjects struct {
	EditableSlice
	ProjectItems map[string]string
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

// ProjectIds returns a slice containing IDs of projects v1 that the issue or a PR has to be linked to.
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
	p, _, err := e.Metadata.ProjectsToIDs(e.Projects.Value)
	return &p, err
}

// ProjectV2Ids returns a pair of slices.
// The first is the projects the item should be added to.
// The second is the projects the items should be removed from.
func (e Editable) ProjectV2Ids() (*[]string, *[]string, error) {
	if !e.Projects.Edited {
		return nil, nil, nil
	}

	// titles of projects to add
	addTitles := set.NewStringSet()
	// titles of projects to remove
	removeTitles := set.NewStringSet()

	if len(e.Projects.Add) != 0 || len(e.Projects.Remove) != 0 {
		// Projects were selected using flags.
		addTitles.AddValues(e.Projects.Add)
		removeTitles.AddValues(e.Projects.Remove)
	} else {
		// Projects were selected interactively.
		addTitles.AddValues(e.Projects.Value)
		addTitles.RemoveValues(e.Projects.Default)
		removeTitles.AddValues(e.Projects.Default)
		removeTitles.RemoveValues(e.Projects.Value)
	}

	var addIds []string
	var removeIds []string
	var err error

	if addTitles.Len() > 0 {
		_, addIds, err = e.Metadata.ProjectsToIDs(addTitles.ToSlice())
		if err != nil {
			return nil, nil, err
		}
	}

	if removeTitles.Len() > 0 {
		_, removeIds, err = e.Metadata.ProjectsToIDs(removeTitles.ToSlice())
		if err != nil {
			return nil, nil, err
		}
	}

	return &addIds, &removeIds, nil
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

// Clone creates a mostly-shallow copy of Editable suitable for use in parallel
// go routines. Fields that would be mutated will be copied.
func (e *Editable) Clone() Editable {
	return Editable{
		Title:     e.Title.clone(),
		Body:      e.Body.clone(),
		Base:      e.Base.clone(),
		Reviewers: e.Reviewers.clone(),
		Assignees: e.Assignees.clone(),
		Labels:    e.Labels.clone(),
		Projects:  e.Projects.clone(),
		Milestone: e.Milestone.clone(),
		// Shallow copy since no mutation.
		Metadata: e.Metadata,
	}
}

func (es *EditableString) clone() EditableString {
	return EditableString{
		Value:   es.Value,
		Default: es.Default,
		Edited:  es.Edited,
		// Shallow copies since no mutation.
		Options: es.Options,
	}
}

func (es *EditableSlice) clone() EditableSlice {
	cpy := EditableSlice{
		Edited:  es.Edited,
		Allowed: es.Allowed,
		// Shallow copies since no mutation.
		Options: es.Options,
		// Copy mutable string slices.
		Add:     make([]string, len(es.Add)),
		Remove:  make([]string, len(es.Remove)),
		Value:   make([]string, len(es.Value)),
		Default: make([]string, len(es.Default)),
	}
	copy(cpy.Add, es.Add)
	copy(cpy.Remove, es.Remove)
	copy(cpy.Value, es.Value)
	copy(cpy.Default, es.Default)
	return cpy
}

func (ep *EditableProjects) clone() EditableProjects {
	return EditableProjects{
		EditableSlice: ep.EditableSlice.clone(),
		ProjectItems:  ep.ProjectItems,
	}
}

type EditPrompter interface {
	Select(string, string, []string) (int, error)
	Input(string, string) (string, error)
	MarkdownEditor(string, string, bool) (string, error)
	MultiSelect(string, []string, []string) ([]int, error)
	Confirm(string, bool) (bool, error)
}

func EditFieldsSurvey(p EditPrompter, editable *Editable, editorCommand string) error {
	var err error
	if editable.Title.Edited {
		editable.Title.Value, err = p.Input("Title", editable.Title.Default)
		if err != nil {
			return err
		}
	}
	if editable.Body.Edited {
		editable.Body.Value, err = p.MarkdownEditor("Body", editable.Body.Default, false)
		if err != nil {
			return err
		}
	}
	if editable.Reviewers.Edited {
		editable.Reviewers.Value, err = multiSelectSurvey(
			p, "Reviewers", editable.Reviewers.Default, editable.Reviewers.Options)
		if err != nil {
			return err
		}
	}
	if editable.Assignees.Edited {
		editable.Assignees.Value, err = multiSelectSurvey(
			p, "Assignees", editable.Assignees.Default, editable.Assignees.Options)
		if err != nil {
			return err
		}
	}
	if editable.Labels.Edited {
		editable.Labels.Add, err = multiSelectSurvey(
			p, "Labels", editable.Labels.Default, editable.Labels.Options)
		if err != nil {
			return err
		}
		for _, prev := range editable.Labels.Default {
			var found bool
			for _, selected := range editable.Labels.Add {
				if prev == selected {
					found = true
					break
				}
			}
			if !found {
				editable.Labels.Remove = append(editable.Labels.Remove, prev)
			}
		}
	}
	if editable.Projects.Edited {
		editable.Projects.Value, err = multiSelectSurvey(
			p, "Projects", editable.Projects.Default, editable.Projects.Options)
		if err != nil {
			return err
		}
	}
	if editable.Milestone.Edited {
		editable.Milestone.Value, err = milestoneSurvey(p, editable.Milestone.Default, editable.Milestone.Options)
		if err != nil {
			return err
		}
	}
	confirm, err := p.Confirm("Submit?", true)
	if err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("Discarding...")
	}

	return nil
}

func FieldsToEditSurvey(p EditPrompter, editable *Editable) error {
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
	results, err := multiSelectSurvey(p, "What would you like to edit?", []string{}, opts)
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
	for _, p := range metadata.Projects {
		projects = append(projects, p.Name)
	}
	for _, p := range metadata.ProjectsV2 {
		projects = append(projects, p.Title)
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

func multiSelectSurvey(p EditPrompter, message string, defaults, options []string) (results []string, err error) {
	if len(options) == 0 {
		return nil, nil
	}

	var selected []int
	selected, err = p.MultiSelect(message, defaults, options)
	if err != nil {
		return
	}

	for _, i := range selected {
		results = append(results, options[i])
	}

	return results, err
}

func milestoneSurvey(p EditPrompter, title string, opts []string) (result string, err error) {
	if len(opts) == 0 {
		return "", nil
	}
	var selected int
	selected, err = p.Select("Milestone", title, opts)
	if err != nil {
		return
	}

	result = opts[selected]
	return
}
