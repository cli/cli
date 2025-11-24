package shared

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/google/shlex"
)

func WithPrAndIssueQueryParams(client *api.Client, baseRepo ghrepo.Interface, baseURL string, state IssueMetadataState, projectsV1Support gh.ProjectsV1Support) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if state.Title != "" {
		q.Set("title", state.Title)
	}
	// We always want to send the body parameter, even if it's empty, to prevent the web interface from
	// applying the default template. Since the user has the option to select a template in the terminal,
	// assume that empty body here means that the user either skipped it or erased its contents.
	q.Set("body", state.Body)
	if len(state.Assignees) > 0 {
		q.Set("assignees", strings.Join(state.Assignees, ","))
	}
	// Set a template parameter if no body parameter is provided e.g. Web Mode
	if len(state.Template) > 0 && len(state.Body) == 0 {
		q.Set("template", state.Template)
	}
	if len(state.Labels) > 0 {
		q.Set("labels", strings.Join(state.Labels, ","))
	}
	if len(state.ProjectTitles) > 0 {
		projectPaths, err := api.ProjectTitlesToPaths(client, baseRepo, state.ProjectTitles, projectsV1Support)
		if err != nil {
			return "", fmt.Errorf("could not add to project: %w", err)
		}
		q.Set("projects", strings.Join(projectPaths, ","))
	}
	if len(state.Milestones) > 0 {
		q.Set("milestone", state.Milestones[0])
	}

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Maximum length of a URL: 8192 bytes
func ValidURL(urlStr string) bool {
	return len(urlStr) < 8192
}

func AddMetadataToIssueParams(client *api.Client, baseRepo ghrepo.Interface, params map[string]interface{}, tb *IssueMetadataState, projectV1Support gh.ProjectsV1Support) error {
	if !tb.HasMetadata() {
		return nil
	}

	// Retrieve minimal information needed to resolve metadata if this was not previously cached from additional metadata survey.
	if tb.MetadataResult == nil {
		input := api.RepoMetadataInput{
			Reviewers: len(tb.Reviewers) > 0,
			TeamReviewers: len(tb.Reviewers) > 0 && slices.ContainsFunc(tb.Reviewers, func(r string) bool {
				return strings.ContainsRune(r, '/')
			}),
			Assignees:      len(tb.Assignees) > 0,
			ActorAssignees: tb.ActorAssignees,
			Labels:         len(tb.Labels) > 0,
			ProjectsV1:     len(tb.ProjectTitles) > 0 && projectV1Support == gh.ProjectsV1Supported,
			ProjectsV2:     len(tb.ProjectTitles) > 0,
			Milestones:     len(tb.Milestones) > 0,
		}

		metadataResult, err := api.RepoMetadata(client, baseRepo, input)
		if err != nil {
			return err
		}
		tb.MetadataResult = metadataResult
	}

	assigneeIDs, err := tb.MetadataResult.MembersToIDs(tb.Assignees)
	if err != nil {
		return fmt.Errorf("could not assign user: %w", err)
	}
	params["assigneeIds"] = assigneeIDs

	labelIDs, err := tb.MetadataResult.LabelsToIDs(tb.Labels)
	if err != nil {
		return fmt.Errorf("could not add label: %w", err)
	}
	params["labelIds"] = labelIDs

	projectIDs, projectV2IDs, err := tb.MetadataResult.ProjectsTitlesToIDs(tb.ProjectTitles)
	if err != nil {
		return fmt.Errorf("could not add to project: %w", err)
	}
	params["projectIds"] = projectIDs
	params["projectV2Ids"] = projectV2IDs

	if len(tb.Milestones) > 0 {
		milestoneID, err := tb.MetadataResult.MilestoneToID(tb.Milestones[0])
		if err != nil {
			return fmt.Errorf("could not add to milestone '%s': %w", tb.Milestones[0], err)
		}
		params["milestoneId"] = milestoneID
	}

	if len(tb.Reviewers) == 0 {
		return nil
	}

	var userReviewers []string
	var teamReviewers []string
	for _, r := range tb.Reviewers {
		if strings.ContainsRune(r, '/') {
			teamReviewers = append(teamReviewers, r)
		} else {
			userReviewers = append(userReviewers, r)
		}
	}

	userReviewerIDs, err := tb.MetadataResult.MembersToIDs(userReviewers)
	if err != nil {
		return fmt.Errorf("could not request reviewer: %w", err)
	}
	params["userReviewerIds"] = userReviewerIDs

	teamReviewerIDs, err := tb.MetadataResult.TeamsToIDs(teamReviewers)
	if err != nil {
		return fmt.Errorf("could not request reviewer: %w", err)
	}
	params["teamReviewerIds"] = teamReviewerIDs

	return nil
}

type FilterOptions struct {
	Assignee   string
	Author     string
	BaseBranch string
	Draft      *bool
	Entity     string
	Fields     []string
	HeadBranch string
	Labels     []string
	Mention    string
	Milestone  string
	Repo       string
	Search     string
	State      string
}

func (opts *FilterOptions) IsDefault() bool {
	if opts.State != "open" {
		return false
	}
	if len(opts.Labels) > 0 {
		return false
	}
	if opts.Assignee != "" {
		return false
	}
	if opts.Author != "" {
		return false
	}
	if opts.BaseBranch != "" {
		return false
	}
	if opts.HeadBranch != "" {
		return false
	}
	if opts.Mention != "" {
		return false
	}
	if opts.Milestone != "" {
		return false
	}
	if opts.Search != "" {
		return false
	}
	return true
}

func ListURLWithQuery(listURL string, options FilterOptions, advancedIssueSearchSyntax bool) (string, error) {
	u, err := url.Parse(listURL)
	if err != nil {
		return "", err
	}

	params := u.Query()
	params.Set("q", SearchQueryBuild(options, advancedIssueSearchSyntax))
	u.RawQuery = params.Encode()

	return u.String(), nil
}

func SearchQueryBuild(options FilterOptions, advancedIssueSearchSyntax bool) string {
	var is, state string
	switch options.State {
	case "open", "closed":
		state = options.State
	case "merged":
		is = "merged"
	}
	query := search.Query{
		Qualifiers: search.Qualifiers{
			Assignee:  options.Assignee,
			Author:    options.Author,
			Base:      options.BaseBranch,
			Draft:     options.Draft,
			Head:      options.HeadBranch,
			Label:     options.Labels,
			Mentions:  options.Mention,
			Milestone: options.Milestone,
			Repo:      []string{options.Repo},
			State:     state,
			Is:        []string{is},
			Type:      options.Entity,
		},
		ImmutableKeywords: options.Search,
	}

	if !advancedIssueSearchSyntax {
		return query.StandardSearchString()
	}
	return query.AdvancedIssueSearchString()
}

func QueryHasStateClause(searchQuery string) bool {
	argv, err := shlex.Split(searchQuery)
	if err != nil {
		return false
	}

	for _, arg := range argv {
		if arg == "is:closed" || arg == "is:merged" || arg == "state:closed" || arg == "state:merged" || strings.HasPrefix(arg, "merged:") || strings.HasPrefix(arg, "closed:") {
			return true
		}
	}

	return false
}

// MeReplacer resolves usages of `@me` to the handle of the currently logged in user.
type MeReplacer struct {
	apiClient *api.Client
	hostname  string
	login     string
}

func NewMeReplacer(apiClient *api.Client, hostname string) *MeReplacer {
	return &MeReplacer{
		apiClient: apiClient,
		hostname:  hostname,
	}
}

func (r *MeReplacer) currentLogin() (string, error) {
	if r.login != "" {
		return r.login, nil
	}
	login, err := api.CurrentLoginName(r.apiClient, r.hostname)
	if err != nil {
		return "", fmt.Errorf("failed resolving `@me` to your user handle: %w", err)
	}
	r.login = login
	return login, nil
}

func (r *MeReplacer) Replace(handle string) (string, error) {
	if handle == "@me" {
		return r.currentLogin()
	}
	return handle, nil
}

func (r *MeReplacer) ReplaceSlice(handles []string) ([]string, error) {
	res := make([]string, len(handles))
	for i, h := range handles {
		var err error
		res[i], err = r.Replace(h)
		if err != nil {
			return nil, err
		}
	}
	return res, nil
}

// CopilotReplacer resolves usages of `@copilot` to either Copilot's login or name.
// Login is generally needed for API calls; name is used when launching web browser.
type CopilotReplacer struct {
	returnLogin bool
}

func NewCopilotReplacer(returnLogin bool) *CopilotReplacer {
	return &CopilotReplacer{
		returnLogin: returnLogin,
	}
}

func (r *CopilotReplacer) replace(handle string) string {
	if !strings.EqualFold(handle, "@copilot") {
		return handle
	}
	if r.returnLogin {
		return api.CopilotActorLogin
	}
	return api.CopilotActorName
}

// ReplaceSlice replaces usages of `@copilot` in a slice with Copilot's login.
func (r *CopilotReplacer) ReplaceSlice(handles []string) []string {
	res := make([]string, len(handles))
	for i, h := range handles {
		res[i] = r.replace(h)
	}
	return res
}
