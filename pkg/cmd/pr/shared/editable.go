package shared

import (
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/set"
)

type Editable struct {
	Title              EditableString
	Body               EditableString
	Base               EditableString
	Reviewers          EditableReviewers
	ReviewerSearchFunc func(string) prompter.MultiSelectSearchResult
	Assignees          EditableAssignees
	AssigneeSearchFunc func(string) prompter.MultiSelectSearchResult
	Labels             EditableSlice
	Projects           EditableProjects
	Milestone          EditableString
	Metadata           api.RepoMetadataResult

	// TODO ApiActorsSupported
	// ApiActorsSupported indicates the host supports actor-based APIs (github.com, ghe.com).
	// When true, mutations use logins directly instead of resolving node IDs.
	// Remove this flag (and collapse to actor-only paths) once GHES supports
	// replaceActorsForAssignable and requestReviewsByLogin mutations.
	ApiActorsSupported bool
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

// EditableAssignees is a special case of EditableSlice.
type EditableAssignees struct {
	EditableSlice
	DefaultLogins []string // For disambiguating actors from display names
}

// EditableReviewers is a special case of EditableSlice.
type EditableReviewers struct {
	EditableSlice
	DefaultLogins []string // For disambiguating actors from display names
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

func (e Editable) AssigneeIds(client *api.Client, repo ghrepo.Interface) (*[]string, error) {
	if !e.Assignees.Edited {
		return nil, nil
	}

	// If assignees came in from command line flags, we need to
	// curate the final list of assignees from the default list.
	if len(e.Assignees.Add) != 0 || len(e.Assignees.Remove) != 0 {
		// TODO ApiActorsSupported
		replacer := NewSpecialAssigneeReplacer(client, repo.RepoHost(), e.ApiActorsSupported, true)

		assigneeSet := set.NewStringSet()

		// This check below is required because in a non-interactive flow,
		// the user gives us a login and not the DisplayName, and when
		// we have actor assignees e.Assignees.Default will contain
		// DisplayNames and not logins (this is to accommodate special actor
		// display names in the interactive flow).
		// So, we need to add the default logins here instead of the DisplayNames.
		// Otherwise, the value the user provided won't be found in the
		// set to be added or removed, causing unexpected behavior.
		// TODO ApiActorsSupported
		if e.ApiActorsSupported {
			assigneeSet.AddValues(e.Assignees.DefaultLogins)
		} else {
			assigneeSet.AddValues(e.Assignees.Default)
		}

		add, err := replacer.ReplaceSlice(e.Assignees.Add)
		if err != nil {
			return nil, err
		}
		assigneeSet.AddValues(add)

		remove, err := replacer.ReplaceSlice(e.Assignees.Remove)
		if err != nil {
			return nil, err
		}
		assigneeSet.RemoveValues(remove)

		e.Assignees.Value = assigneeSet.ToSlice()
	}
	a, err := e.Metadata.MembersToIDs(e.Assignees.Value)
	return &a, err
}

// AssigneeLogins computes the final list of assignee logins from the current
// defaults plus any Add/Remove operations. Unlike AssigneeIds, this does not
// resolve logins to node IDs, and is used on github.com where the
// ReplaceActorsForAssignable mutation accepts logins directly.
func (e Editable) AssigneeLogins(client *api.Client, repo ghrepo.Interface) ([]string, error) {
	if !e.Assignees.Edited {
		return nil, nil
	}

	if len(e.Assignees.Add) != 0 || len(e.Assignees.Remove) != 0 {
		replacer := NewSpecialAssigneeReplacer(client, repo.RepoHost(), true, true)

		assigneeSet := set.NewStringSet()
		assigneeSet.AddValues(e.Assignees.DefaultLogins)

		add, err := replacer.ReplaceSlice(e.Assignees.Add)
		if err != nil {
			return nil, err
		}
		assigneeSet.AddValues(add)

		remove, err := replacer.ReplaceSlice(e.Assignees.Remove)
		if err != nil {
			return nil, err
		}
		assigneeSet.RemoveValues(remove)

		e.Assignees.Value = assigneeSet.ToSlice()
	}

	return e.Assignees.Value, nil
}

// SpecialAssigneeReplacer expands special assignee names (@me, Copilot actors)
// in login slices. Use NewSpecialAssigneeReplacer to create one.
type SpecialAssigneeReplacer struct {
	meReplacer      *MeReplacer
	copilotReplacer *CopilotReplacer
	actorAssignees  bool
}

// NewSpecialAssigneeReplacer creates a replacer that expands @me and (when
// actorAssignees is true) Copilot actor names in assignee slices.
// copilotUseLogin controls whether Copilot actors are replaced with their
// login (true) or display name (false, used for web mode).
func NewSpecialAssigneeReplacer(client *api.Client, host string, actorAssignees bool, copilotUseLogin bool) *SpecialAssigneeReplacer {
	return &SpecialAssigneeReplacer{
		meReplacer:      NewMeReplacer(client, host),
		copilotReplacer: NewCopilotReplacer(copilotUseLogin),
		actorAssignees:  actorAssignees,
	}
}

func (r *SpecialAssigneeReplacer) ReplaceSlice(logins []string) ([]string, error) {
	replaced, err := r.meReplacer.ReplaceSlice(logins)
	if err != nil {
		return nil, err
	}
	if r.actorAssignees {
		replaced = r.copilotReplacer.ReplaceSlice(replaced)
	}
	return replaced, nil
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
	p, _, err := e.Metadata.ProjectsTitlesToIDs(e.Projects.Value)
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
		_, addIds, err = e.Metadata.ProjectsTitlesToIDs(addTitles.ToSlice())
		if err != nil {
			return nil, nil, err
		}
	}

	if removeTitles.Len() > 0 {
		_, removeIds, err = e.Metadata.ProjectsTitlesToIDs(removeTitles.ToSlice())
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
		Title:              e.Title.clone(),
		Body:               e.Body.clone(),
		Base:               e.Base.clone(),
		Reviewers:          e.Reviewers.clone(),
		ReviewerSearchFunc: e.ReviewerSearchFunc,
		Assignees:          e.Assignees.clone(),
		AssigneeSearchFunc: e.AssigneeSearchFunc,
		Labels:             e.Labels.clone(),
		Projects:           e.Projects.clone(),
		Milestone:          e.Milestone.clone(),
		ApiActorsSupported: e.ApiActorsSupported,
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

func (ea *EditableAssignees) clone() EditableAssignees {
	return EditableAssignees{
		EditableSlice: ea.EditableSlice.clone(),
		DefaultLogins: ea.DefaultLogins,
	}
}

func (er *EditableReviewers) clone() EditableReviewers {
	return EditableReviewers{
		EditableSlice: er.EditableSlice.clone(),
		DefaultLogins: er.DefaultLogins,
	}
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
	MultiSelectWithSearch(prompt, searchPrompt string, defaults []string, persistentOptions []string, searchFunc func(string) prompter.MultiSelectSearchResult) ([]string, error)
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
		if editable.ReviewerSearchFunc != nil {
			editable.Reviewers.Options = []string{}
			editable.Reviewers.Value, err = p.MultiSelectWithSearch(
				"Reviewers",
				"Search reviewers",
				editable.Reviewers.DefaultLogins,
				// No persistent options - teams are included in search results
				[]string{},
				editable.ReviewerSearchFunc)
			if err != nil {
				return err
			}
		} else {
			editable.Reviewers.Value, err = multiSelectSurvey(
				p, "Reviewers", editable.Reviewers.Default, editable.Reviewers.Options)
			if err != nil {
				return err
			}
		}
	}
	if editable.Assignees.Edited {
		if editable.AssigneeSearchFunc != nil {
			editable.Assignees.Options = []string{}
			editable.Assignees.Value, err = p.MultiSelectWithSearch(
				"Assignees",
				"Search assignees",
				editable.Assignees.DefaultLogins,
				// No persistent options required here as teams cannot be assignees.
				[]string{},
				editable.AssigneeSearchFunc)
			if err != nil {
				return err
			}
		} else {
			editable.Assignees.Value, err = multiSelectSurvey(
				p, "Assignees", editable.Assignees.Default, editable.Assignees.Options)
			if err != nil {
				return err
			}
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

func FetchOptions(client *api.Client, repo ghrepo.Interface, editable *Editable, projectV1Support gh.ProjectsV1Support) error {
	// Determine whether to fetch organization teams and reviewers.
	// Interactive reviewer editing (Edited true, but no Add/Remove slices) still needs
	// team data for selection UI. For non-interactive flows, we never need to fetch teams
	// as the REST API accepts team slugs directly.
	// If we have a search func, we don't need to fetch teams/reviewers since we
	// assume that will be done dynamically in the prompting flow.
	teamReviewers := false
	fetchReviewers := false
	if editable.Reviewers.Edited {
		// This is likely an interactive flow since edited is set but no mutations to
		// Add/Remove slices, so we need to load the teams and reviewers.
		// However, if we have a search func, skip fetching as it will be done dynamically.
		if len(editable.Reviewers.Add) == 0 && len(editable.Reviewers.Remove) == 0 && editable.ReviewerSearchFunc == nil {
			teamReviewers = true
			fetchReviewers = true
		}
		// Note: Non-interactive flows (with Add/Remove) don't need to fetch reviewers/teams
		// because the APIs in use for both GHES and GitHub.com accept user logins and team slugs directly.
	}

	fetchAssignees := false
	if editable.Assignees.Edited {
		// Similar as above, this is likely an interactive flow if no Add/Remove slices are set.
		// If we have a search func, we don't need to fetch assignees since we
		// assume that will be done dynamically in the prompting flow.
		if len(editable.Assignees.Add) == 0 && len(editable.Assignees.Remove) == 0 && editable.AssigneeSearchFunc == nil {
			fetchAssignees = true
		}
		// For non-interactive Add/Remove operations, we only need to fetch assignees
		// on GHES where ID resolution is required. On github.com (ApiActorsSupported),
		// logins are passed directly to the mutation.
		// TODO ApiActorsSupported
		if (len(editable.Assignees.Add) > 0 || len(editable.Assignees.Remove) > 0) && !editable.ApiActorsSupported {
			fetchAssignees = true
		}
	}

	input := api.RepoMetadataInput{
		Reviewers:          fetchReviewers,
		TeamReviewers:      teamReviewers,
		Assignees:          fetchAssignees,
		ApiActorsSupported: editable.ApiActorsSupported,
		Labels:             editable.Labels.Edited,
		ProjectsV1:         editable.Projects.Edited && projectV1Support == gh.ProjectsV1Supported,
		ProjectsV2:         editable.Projects.Edited,
		Milestones:         editable.Milestone.Edited,
	}
	metadata, err := api.RepoMetadata(client, repo, input)
	if err != nil {
		return err
	}

	var users []string
	for _, u := range metadata.AssignableUsers {
		users = append(users, u.Login())
	}
	var actors []string
	for _, a := range metadata.AssignableActors {
		actors = append(actors, a.DisplayName())
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
	// TODO ApiActorsSupported
	if editable.ApiActorsSupported {
		editable.Assignees.Options = actors
	} else {
		editable.Assignees.Options = users
	}
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

// AssigneeSearchFunc returns a search function for MultiSelectWithSearch that
// dynamically fetches assignable actors for the given assignable (Issue/PR) node ID.
func AssigneeSearchFunc(apiClient *api.Client, repo ghrepo.Interface, assignableID string) func(string) prompter.MultiSelectSearchResult {
	return func(input string) prompter.MultiSelectSearchResult {
		actors, count, err := api.SuggestedAssignableActors(apiClient, repo, assignableID, input)
		if err != nil {
			return prompter.MultiSelectSearchResult{Err: err}
		}
		return actorsToSearchResult(actors, count)
	}
}

// RepoAssigneeSearchFunc returns a search function for MultiSelectWithSearch that
// dynamically fetches assignable actors at the repository level. Used during create
// flows where no issue/PR node ID exists yet.
func RepoAssigneeSearchFunc(apiClient *api.Client, repo ghrepo.Interface) func(string) prompter.MultiSelectSearchResult {
	return func(input string) prompter.MultiSelectSearchResult {
		actors, count, err := api.SearchRepoAssignableActors(apiClient, repo, input)
		if err != nil {
			return prompter.MultiSelectSearchResult{Err: err}
		}
		return actorsToSearchResult(actors, count)
	}
}

func actorsToSearchResult(actors []api.AssignableActor, totalCount int) prompter.MultiSelectSearchResult {
	logins := make([]string, 0, len(actors))
	displayNames := make([]string, 0, len(actors))

	for _, a := range actors {
		if a.Login() == "" {
			continue
		}
		logins = append(logins, a.Login())
		if a.DisplayName() != "" {
			displayNames = append(displayNames, a.DisplayName())
		} else {
			displayNames = append(displayNames, a.Login())
		}
	}
	return prompter.MultiSelectSearchResult{
		Keys:        logins,
		Labels:      displayNames,
		MoreResults: totalCount,
	}
}
