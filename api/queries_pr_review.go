package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/shurcooL/githubv4"
)

type PullRequestReviewState int

const (
	ReviewApprove PullRequestReviewState = iota
	ReviewRequestChanges
	ReviewComment
)

type PullRequestReviewInput struct {
	Body  string
	State PullRequestReviewState
}

type PullRequestReviews struct {
	Nodes    []PullRequestReview
	PageInfo struct {
		HasNextPage bool
		EndCursor   string
	}
	TotalCount int
}

type PullRequestReview struct {
	ID                  string         `json:"id"`
	Author              CommentAuthor  `json:"author"`
	AuthorAssociation   string         `json:"authorAssociation"`
	Body                string         `json:"body"`
	SubmittedAt         *time.Time     `json:"submittedAt"`
	IncludesCreatedEdit bool           `json:"includesCreatedEdit"`
	ReactionGroups      ReactionGroups `json:"reactionGroups"`
	State               string         `json:"state"`
	URL                 string         `json:"url,omitempty"`
	Commit              Commit         `json:"commit"`
}

func (prr PullRequestReview) Identifier() string {
	return prr.ID
}

func (prr PullRequestReview) AuthorLogin() string {
	return prr.Author.DisplayName()
}

func (prr PullRequestReview) Association() string {
	return prr.AuthorAssociation
}

func (prr PullRequestReview) Content() string {
	return prr.Body
}

func (prr PullRequestReview) Created() time.Time {
	if prr.SubmittedAt == nil {
		return time.Time{}
	}
	return *prr.SubmittedAt
}

func (prr PullRequestReview) HiddenReason() string {
	return ""
}

func (prr PullRequestReview) IsEdited() bool {
	return prr.IncludesCreatedEdit
}

func (prr PullRequestReview) IsHidden() bool {
	return false
}

func (prr PullRequestReview) Link() string {
	return prr.URL
}

func (prr PullRequestReview) Reactions() ReactionGroups {
	return prr.ReactionGroups
}

func (prr PullRequestReview) Status() string {
	return prr.State
}

type PullRequestReviewStatus struct {
	ChangesRequested bool
	Approved         bool
	ReviewRequired   bool
}

func (pr *PullRequest) ReviewStatus() PullRequestReviewStatus {
	var status PullRequestReviewStatus
	switch pr.ReviewDecision {
	case "CHANGES_REQUESTED":
		status.ChangesRequested = true
	case "APPROVED":
		status.Approved = true
	case "REVIEW_REQUIRED":
		status.ReviewRequired = true
	}
	return status
}

func (pr *PullRequest) DisplayableReviews() PullRequestReviews {
	published := []PullRequestReview{}
	for _, prr := range pr.Reviews.Nodes {
		//Dont display pending reviews
		//Dont display commenting reviews without top level comment body
		if prr.State != "PENDING" && !(prr.State == "COMMENTED" && prr.Body == "") {
			published = append(published, prr)
		}
	}
	return PullRequestReviews{Nodes: published, TotalCount: len(published)}
}

type ReviewRequests struct {
	Nodes []struct {
		RequestedReviewer RequestedReviewer
	}
}

type RequestedReviewer struct {
	TypeName     string `json:"__typename"`
	Login        string `json:"login"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Organization struct {
		Login string `json:"login"`
	} `json:"organization"`
}

const teamTypeName = "Team"
const botTypeName = "Bot"
const userTypeName = "User"

func (r RequestedReviewer) LoginOrSlug() string {
	if r.TypeName == teamTypeName {
		return fmt.Sprintf("%s/%s", r.Organization.Login, r.Slug)
	}
	return r.Login
}

// DisplayName returns a user-friendly name for the reviewer via actorDisplayName.
// Teams are handled separately as "org/slug".
func (r RequestedReviewer) DisplayName() string {
	if r.TypeName == teamTypeName {
		return fmt.Sprintf("%s/%s", r.Organization.Login, r.Slug)
	}
	return actorDisplayName(r.TypeName, r.Login, r.Name)
}

func (r ReviewRequests) Logins() []string {
	logins := make([]string, len(r.Nodes))
	for i, r := range r.Nodes {
		logins[i] = r.RequestedReviewer.LoginOrSlug()
	}
	return logins
}

// DisplayNames returns user-friendly display names for all requested reviewers.
func (r ReviewRequests) DisplayNames() []string {
	names := make([]string, len(r.Nodes))
	for i, r := range r.Nodes {
		names[i] = r.RequestedReviewer.DisplayName()
	}
	return names
}

// ReviewerCandidate represents a potential reviewer for a pull request.
// This can be a User, Bot, or Team. It mirrors AssignableActor but adds
// team support (teams can review but not be assigned) and drops the ID method.
// ReviewerUser and ReviewerBot are thin wrappers around AssignableUser and
// AssignableBot that satisfy this interface.
type ReviewerCandidate interface {
	DisplayName() string
	Login() string

	sealedReviewerCandidate()
}

// ReviewerUser is a user who can review a pull request.
type ReviewerUser struct {
	AssignableUser
}

func NewReviewerUser(login, name string) ReviewerUser {
	return ReviewerUser{
		AssignableUser: NewAssignableUser("", login, name),
	}
}

func (r ReviewerUser) sealedReviewerCandidate() {}

// ReviewerBot is a bot who can review a pull request.
type ReviewerBot struct {
	AssignableBot
}

func NewReviewerBot(login string) ReviewerBot {
	return ReviewerBot{
		AssignableBot: NewAssignableBot("", login),
	}
}

func (b ReviewerBot) DisplayName() string {
	return actorDisplayName("Bot", b.login, "")
}

func (r ReviewerBot) sealedReviewerCandidate() {}

// ReviewerTeam is a team that can review a pull request.
type ReviewerTeam struct {
	org      string
	teamSlug string
}

// NewReviewerTeam creates a new ReviewerTeam.
func NewReviewerTeam(orgName, teamSlug string) ReviewerTeam {
	return ReviewerTeam{org: orgName, teamSlug: teamSlug}
}

func (r ReviewerTeam) DisplayName() string {
	return fmt.Sprintf("%s/%s", r.org, r.teamSlug)
}

func (r ReviewerTeam) Login() string {
	return fmt.Sprintf("%s/%s", r.org, r.teamSlug)
}

func (r ReviewerTeam) Slug() string {
	return r.teamSlug
}

func (r ReviewerTeam) sealedReviewerCandidate() {}

func AddReview(client *Client, repo ghrepo.Interface, pr *PullRequest, input *PullRequestReviewInput) error {
	var mutation struct {
		AddPullRequestReview struct {
			ClientMutationID string
		} `graphql:"addPullRequestReview(input:$input)"`
	}

	state := githubv4.PullRequestReviewEventComment
	switch input.State {
	case ReviewApprove:
		state = githubv4.PullRequestReviewEventApprove
	case ReviewRequestChanges:
		state = githubv4.PullRequestReviewEventRequestChanges
	}

	body := githubv4.String(input.Body)
	variables := map[string]any{
		"input": githubv4.AddPullRequestReviewInput{
			PullRequestID: pr.ID,
			Event:         &state,
			Body:          &body,
		},
	}

	return client.Mutate(repo.RepoHost(), "PullRequestReviewAdd", &mutation, variables)
}

// AddPullRequestReviews adds the given user and team reviewers to a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func AddPullRequestReviews(client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
	if users == nil {
		users = []string{}
	}

	path, err := safeurl.JoinPath(
		"repos",
		repo.RepoOwner(),
		repo.RepoName(),
		"pulls",
		strconv.Itoa(prNumber),
		"requested_reviewers",
	)
	if err != nil {
		return err
	}
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	return client.REST(repo.RepoHost(), "POST", path.String(), buf, nil)
}

// RemovePullRequestReviews removes requested reviewers from a pull request using the REST API.
// Team identifiers can be in "org/slug" format.
func RemovePullRequestReviews(client *Client, repo ghrepo.Interface, prNumber int, users, teams []string) error {
	if len(users) == 0 && len(teams) == 0 {
		return nil
	}

	// The API requires empty arrays instead of null values
	if users == nil {
		users = []string{}
	}

	path, err := safeurl.JoinPath(
		"repos",
		repo.RepoOwner(),
		repo.RepoName(),
		"pulls",
		strconv.Itoa(prNumber),
		"requested_reviewers",
	)
	if err != nil {
		return err
	}
	body := struct {
		Reviewers     []string `json:"reviewers"`
		TeamReviewers []string `json:"team_reviewers"`
	}{
		Reviewers:     users,
		TeamReviewers: extractTeamSlugs(teams),
	}
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return err
	}
	// The endpoint responds with the updated pull request object; we don't need it here.
	return client.REST(repo.RepoHost(), "DELETE", path.String(), buf, nil)
}

// RequestReviewsByLogin sets requested reviewers on a pull request using the GraphQL mutation.
// This mutation replaces existing reviewers with the provided set unless union is true.
// Only available on github.com, not GHES.
// Bot logins should be passed without the [bot] suffix; it is appended automatically.
// Team slugs must be in the format "org/team-slug".
// When union is false (replace mode), passing empty slices will remove all reviewers.
func RequestReviewsByLogin(client *Client, repo ghrepo.Interface, prID string, userLogins, botLogins, teamSlugs []string, union bool) error {
	// In union mode (additive), nothing to do if all lists are empty.
	// In replace mode, we may still need to call the mutation to clear reviewers.
	if union && len(userLogins) == 0 && len(botLogins) == 0 && len(teamSlugs) == 0 {
		return nil
	}

	var mutation struct {
		RequestReviewsByLogin struct {
			ClientMutationId string `graphql:"clientMutationId"`
		} `graphql:"requestReviewsByLogin(input: $input)"`
	}

	type RequestReviewsByLoginInput struct {
		PullRequestID githubv4.ID        `json:"pullRequestId"`
		UserLogins    *[]githubv4.String `json:"userLogins,omitempty"`
		BotLogins     *[]githubv4.String `json:"botLogins,omitempty"`
		TeamSlugs     *[]githubv4.String `json:"teamSlugs,omitempty"`
		Union         githubv4.Boolean   `json:"union"`
	}

	input := RequestReviewsByLoginInput{
		PullRequestID: githubv4.ID(prID),
		Union:         githubv4.Boolean(union),
	}

	userLoginValues := toGitHubV4Strings(userLogins, "")
	input.UserLogins = &userLoginValues

	// Bot logins require the [bot] suffix for the mutation
	botLoginValues := toGitHubV4Strings(botLogins, "[bot]")
	input.BotLogins = &botLoginValues

	teamSlugValues := toGitHubV4Strings(teamSlugs, "")
	input.TeamSlugs = &teamSlugValues

	variables := map[string]any{
		"input": input,
	}

	return client.Mutate(repo.RepoHost(), "RequestReviewsByLogin", &mutation, variables)
}

// SuggestedReviewerActors fetches suggested reviewers for a pull request.
// It combines results from three sources using a cascading quota system:
// - suggestedReviewerActors - suggested based on PR activity (base quota: 5)
// - repository collaborators - all collaborators (base quota: 5 + unfilled from suggestions)
// - organization teams - all teams for org repos (base quota: 5 + unfilled from collaborators)
//
// This ensures we show up to 15 total candidates, with each source filling any
// unfilled quota from the previous source. Results are deduplicated.
// Returns the candidates, a MoreResults count, and an error.
func SuggestedReviewerActors(client *Client, repo ghrepo.Interface, prID string, query string) ([]ReviewerCandidate, int, error) {
	// Fetch 10 from each source to allow cascading quota to fill from available results.
	// Organization teams are fetched via repository.owner inline fragment, which
	// gracefully returns empty data for personal (User-owned) repos.
	// We also fetch unfiltered total counts via aliases for the "X more" display.
	type responseData struct {
		Node struct {
			PullRequest struct {
				Author struct {
					Login string
				}
				SuggestedActors struct {
					Nodes []struct {
						IsAuthor    bool
						IsCommenter bool
						Reviewer    struct {
							TypeName string `graphql:"__typename"`
							User     struct {
								Login string
								Name  string
							} `graphql:"... on User"`
							Bot struct {
								Login string
							} `graphql:"... on Bot"`
						}
					}
				} `graphql:"suggestedReviewerActors(first: 10, query: $query)"`
			} `graphql:"... on PullRequest"`
		} `graphql:"node(id: $id)"`
		Repository struct {
			Owner struct {
				TypeName     string `graphql:"__typename"`
				Organization struct {
					Teams struct {
						Nodes []struct {
							Slug string
						}
					} `graphql:"teams(first: 10, query: $query)"`
					TeamsTotalCount struct {
						TotalCount int
					} `graphql:"teamsTotalCount: teams(first: 0)"`
				} `graphql:"... on Organization"`
			}
			Collaborators struct {
				Nodes []struct {
					Login string
					Name  string
				}
			} `graphql:"collaborators(first: 10, query: $query)"`
			CollaboratorsTotalCount struct {
				TotalCount int
			} `graphql:"collaboratorsTotalCount: collaborators(first: 0)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]any{
		"id":    githubv4.ID(prID),
		"query": githubv4.String(query),
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	var result responseData
	err := client.Query(repo.RepoHost(), "SuggestedReviewerActors", &result, variables)
	if err != nil {
		return nil, 0, err
	}

	// Build candidates using cascading quota logic:
	// Each source has a base quota of 5, plus any unfilled quota from previous sources.
	// This ensures we show up to 15 total candidates, filling gaps when earlier sources have fewer.
	// Pre-seed seen with the PR author since you cannot review your own PR.
	seen := make(map[string]bool)
	if authorLogin := result.Node.PullRequest.Author.Login; authorLogin != "" {
		seen[authorLogin] = true
	}
	var candidates []ReviewerCandidate
	const baseQuota = 5

	// Suggested reviewers (excluding author)
	suggestionsAdded := 0
	for _, n := range result.Node.PullRequest.SuggestedActors.Nodes {
		if suggestionsAdded >= baseQuota {
			break
		}
		if n.IsAuthor {
			continue
		}
		var candidate ReviewerCandidate
		var login string
		if n.Reviewer.TypeName == "User" && n.Reviewer.User.Login != "" {
			login = n.Reviewer.User.Login
			candidate = NewReviewerUser(login, n.Reviewer.User.Name)
		} else if n.Reviewer.TypeName == "Bot" && n.Reviewer.Bot.Login != "" {
			login = n.Reviewer.Bot.Login
			candidate = NewReviewerBot(login)
		} else {
			continue
		}
		if !seen[login] {
			seen[login] = true
			candidates = append(candidates, candidate)
			suggestionsAdded++
		}
	}

	// Collaborators: quota = base + unfilled from suggestions
	collaboratorsQuota := baseQuota + (baseQuota - suggestionsAdded)
	collaboratorsAdded := 0
	for _, c := range result.Repository.Collaborators.Nodes {
		if collaboratorsAdded >= collaboratorsQuota {
			break
		}
		if c.Login == "" {
			continue
		}
		if !seen[c.Login] {
			seen[c.Login] = true
			candidates = append(candidates, NewReviewerUser(c.Login, c.Name))
			collaboratorsAdded++
		}
	}

	// Teams: quota = base + unfilled from collaborators
	teamsQuota := baseQuota + (collaboratorsQuota - collaboratorsAdded)
	teamsAdded := 0
	ownerName := repo.RepoOwner()
	for _, t := range result.Repository.Owner.Organization.Teams.Nodes {
		if teamsAdded >= teamsQuota {
			break
		}
		if t.Slug == "" {
			continue
		}
		teamLogin := fmt.Sprintf("%s/%s", ownerName, t.Slug)
		if !seen[teamLogin] {
			seen[teamLogin] = true
			candidates = append(candidates, NewReviewerTeam(ownerName, t.Slug))
			teamsAdded++
		}
	}

	// MoreResults uses unfiltered total counts (teams will be 0 for personal repos)
	moreResults := result.Repository.CollaboratorsTotalCount.TotalCount + result.Repository.Owner.Organization.TeamsTotalCount.TotalCount

	return candidates, moreResults, nil
}

// SuggestedReviewerActorsForRepo fetches potential reviewers for a repository.
// Unlike SuggestedReviewerActors, this doesn't require an existing PR - used for gh pr create.
// It combines results from two sources using a cascading quota system:
// - repository collaborators (base quota: 5)
// - organization teams (base quota: 5 + unfilled from collaborators)
//
// This ensures we show up to 10 total candidates, with each source filling any
// unfilled quota from the previous source. Results are deduplicated.
// Returns the candidates, a MoreResults count, and an error.
func SuggestedReviewerActorsForRepo(client *Client, repo ghrepo.Interface, query string) ([]ReviewerCandidate, int, error) {
	type responseData struct {
		Viewer struct {
			Login string
		}
		Repository struct {
			// HACK: There's no repo-level API to check Copilot reviewer eligibility,
			// so we piggyback on an open PR's suggestedReviewerActors to detect
			// whether Copilot is available as a reviewer for this repository.
			PullRequests struct {
				Nodes []struct {
					SuggestedActors struct {
						Nodes []struct {
							Reviewer struct {
								TypeName string `graphql:"__typename"`
								Bot      struct {
									Login string
								} `graphql:"... on Bot"`
							}
						}
					} `graphql:"suggestedReviewerActors(first: 10)"`
				}
			} `graphql:"pullRequests(first: 1, states: [OPEN])"`
			Owner struct {
				TypeName     string `graphql:"__typename"`
				Organization struct {
					Teams struct {
						Nodes []struct {
							Slug string
						}
					} `graphql:"teams(first: 10, query: $query)"`
					TeamsTotalCount struct {
						TotalCount int
					} `graphql:"teamsTotalCount: teams(first: 0)"`
				} `graphql:"... on Organization"`
			}
			Collaborators struct {
				Nodes []struct {
					Login string
					Name  string
				}
			} `graphql:"collaborators(first: 10, query: $query)"`
			CollaboratorsTotalCount struct {
				TotalCount int
			} `graphql:"collaboratorsTotalCount: collaborators(first: 0)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]any{
		"query": githubv4.String(query),
		"owner": githubv4.String(repo.RepoOwner()),
		"name":  githubv4.String(repo.RepoName()),
	}

	var result responseData
	err := client.Query(repo.RepoHost(), "SuggestedReviewerActorsForRepo", &result, variables)
	if err != nil {
		return nil, 0, err
	}

	// Build candidates using cascading quota logic.
	// Pre-seed seen with the current user to exclude them from results
	// since you cannot review your own PR.
	seen := make(map[string]bool)
	if result.Viewer.Login != "" {
		seen[result.Viewer.Login] = true
	}
	var candidates []ReviewerCandidate
	const baseQuota = 5

	// Check for Copilot availability from open PR's suggested reviewers
	for _, pr := range result.Repository.PullRequests.Nodes {
		for _, actor := range pr.SuggestedActors.Nodes {
			if actor.Reviewer.TypeName == "Bot" && actor.Reviewer.Bot.Login == CopilotReviewerLogin {
				candidates = append(candidates, NewReviewerBot(CopilotReviewerLogin))
				seen[CopilotReviewerLogin] = true
				break
			}
		}
	}

	// Collaborators
	collaboratorsAdded := 0
	for _, c := range result.Repository.Collaborators.Nodes {
		if collaboratorsAdded >= baseQuota {
			break
		}
		if c.Login == "" {
			continue
		}
		if !seen[c.Login] {
			seen[c.Login] = true
			candidates = append(candidates, NewReviewerUser(c.Login, c.Name))
			collaboratorsAdded++
		}
	}

	// Teams: quota = base + unfilled from collaborators
	teamsQuota := baseQuota + (baseQuota - collaboratorsAdded)
	teamsAdded := 0
	ownerName := repo.RepoOwner()
	for _, t := range result.Repository.Owner.Organization.Teams.Nodes {
		if teamsAdded >= teamsQuota {
			break
		}
		if t.Slug == "" {
			continue
		}
		teamLogin := fmt.Sprintf("%s/%s", ownerName, t.Slug)
		if !seen[teamLogin] {
			seen[teamLogin] = true
			candidates = append(candidates, NewReviewerTeam(ownerName, t.Slug))
			teamsAdded++
		}
	}

	// MoreResults uses unfiltered total counts (teams will be 0 for personal repos)
	moreResults := result.Repository.CollaboratorsTotalCount.TotalCount + result.Repository.Owner.Organization.TeamsTotalCount.TotalCount

	return candidates, moreResults, nil
}

// extractTeamSlugs extracts just the slug portion from team identifiers.
// Team identifiers can be in "org/slug" format; this returns just the slug.
func extractTeamSlugs(teams []string) []string {
	slugs := make([]string, 0, len(teams))
	for _, t := range teams {
		if t == "" {
			continue
		}
		s := strings.SplitN(t, "/", 2)
		slugs = append(slugs, s[len(s)-1])
	}
	return slugs
}

// toGitHubV4Strings converts a string slice to a githubv4.String slice,
// optionally appending a suffix to each element.
func toGitHubV4Strings(strs []string, suffix string) []githubv4.String {
	result := make([]githubv4.String, len(strs))
	for i, s := range strs {
		result[i] = githubv4.String(s + suffix)
	}
	return result
}
