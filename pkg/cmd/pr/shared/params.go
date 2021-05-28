package shared

import (
	"fmt"
	"github.com/google/shlex"
	"net/url"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/githubsearch"
)

func WithPrAndIssueQueryParams(client *api.Client, baseRepo ghrepo.Interface, baseURL string, state IssueMetadataState) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if state.Title != "" {
		q.Set("title", state.Title)
	}
	if state.Body != "" {
		q.Set("body", state.Body)
	}
	if len(state.Assignees) > 0 {
		q.Set("assignees", strings.Join(state.Assignees, ","))
	}
	if len(state.Labels) > 0 {
		q.Set("labels", strings.Join(state.Labels, ","))
	}
	if len(state.Projects) > 0 {
		projectPaths, err := api.ProjectNamesToPaths(client, baseRepo, state.Projects)
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

// Ensure that tb.MetadataResult object exists and contains enough pre-fetched API data to be able
// to resolve all object listed in tb to GraphQL IDs.
func fillMetadata(client *api.Client, baseRepo ghrepo.Interface, tb *IssueMetadataState) error {
	resolveInput := api.RepoResolveInput{}

	if len(tb.Assignees) > 0 && (tb.MetadataResult == nil || len(tb.MetadataResult.AssignableUsers) == 0) {
		resolveInput.Assignees = tb.Assignees
	}

	if len(tb.Reviewers) > 0 && (tb.MetadataResult == nil || len(tb.MetadataResult.AssignableUsers) == 0) {
		resolveInput.Reviewers = tb.Reviewers
	}

	if len(tb.Labels) > 0 && (tb.MetadataResult == nil || len(tb.MetadataResult.Labels) == 0) {
		resolveInput.Labels = tb.Labels
	}

	if len(tb.Projects) > 0 && (tb.MetadataResult == nil || len(tb.MetadataResult.Projects) == 0) {
		resolveInput.Projects = tb.Projects
	}

	if len(tb.Milestones) > 0 && (tb.MetadataResult == nil || len(tb.MetadataResult.Milestones) == 0) {
		resolveInput.Milestones = tb.Milestones
	}

	metadataResult, err := api.RepoResolveMetadataIDs(client, baseRepo, resolveInput)
	if err != nil {
		return err
	}

	if tb.MetadataResult == nil {
		tb.MetadataResult = metadataResult
	} else {
		tb.MetadataResult.Merge(metadataResult)
	}

	return nil
}

func AddMetadataToIssueParams(client *api.Client, baseRepo ghrepo.Interface, params map[string]interface{}, tb *IssueMetadataState) error {
	if !tb.HasMetadata() {
		return nil
	}

	if err := fillMetadata(client, baseRepo, tb); err != nil {
		return err
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

	projectIDs, err := tb.MetadataResult.ProjectsToIDs(tb.Projects)
	if err != nil {
		return fmt.Errorf("could not add to project: %w", err)
	}
	params["projectIds"] = projectIDs

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
	Entity     string
	State      string
	Assignee   string
	Labels     []string
	Author     string
	BaseBranch string
	Mention    string
	Milestone  string
	Search     string

	Fields []string
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

func ListURLWithQuery(listURL string, options FilterOptions) (string, error) {
	u, err := url.Parse(listURL)
	if err != nil {
		return "", err
	}

	params := u.Query()
	params.Set("q", SearchQueryBuild(options))
	u.RawQuery = params.Encode()

	return u.String(), nil
}

func SearchQueryBuild(options FilterOptions) string {
	q := githubsearch.NewQuery()
	switch options.Entity {
	case "issue":
		q.SetType(githubsearch.Issue)
	case "pr":
		q.SetType(githubsearch.PullRequest)
	}

	switch options.State {
	case "open":
		q.SetState(githubsearch.Open)
	case "closed":
		q.SetState(githubsearch.Closed)
	case "merged":
		q.SetState(githubsearch.Merged)
	}

	if options.Assignee != "" {
		q.AssignedTo(options.Assignee)
	}
	for _, label := range options.Labels {
		q.AddLabel(label)
	}
	if options.Author != "" {
		q.AuthoredBy(options.Author)
	}
	if options.BaseBranch != "" {
		q.SetBaseBranch(options.BaseBranch)
	}
	if options.Mention != "" {
		q.Mentions(options.Mention)
	}
	if options.Milestone != "" {
		q.InMilestone(options.Milestone)
	}
	if options.Search != "" {
		q.AddQuery(options.Search)
	}

	return q.String()
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
