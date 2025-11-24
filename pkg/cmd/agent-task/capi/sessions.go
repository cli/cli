package capi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/shurcooL/githubv4"
	"github.com/vmihailenco/msgpack/v5"
)

const AgentsHomeURL = "https://github.com/copilot/agents"

var defaultSessionsPerPage = 50

var ErrSessionNotFound = errors.New("not found")

// session is an in-flight agent task
type session struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	UserID           int64     `json:"user_id"`
	AgentID          int64     `json:"agent_id"`
	Logs             string    `json:"logs"`
	State            string    `json:"state"`
	OwnerID          uint64    `json:"owner_id"`
	RepoID           uint64    `json:"repo_id"`
	ResourceType     string    `json:"resource_type"`
	ResourceID       int64     `json:"resource_id"`
	ResourceGlobalID string    `json:"resource_global_id"`
	LastUpdatedAt    time.Time `json:"last_updated_at,omitempty"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
	CompletedAt      time.Time `json:"completed_at,omitempty"`
	EventURL         string    `json:"event_url"`
	EventType        string    `json:"event_type"`
	PremiumRequests  float64   `json:"premium_requests"`
	WorkflowRunID    uint64    `json:"workflow_run_id,omitempty"`
	Error            *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// A shim of a full pull request because looking up by node ID
// using the full api.PullRequest type fails on unions (actors)
type sessionPullRequest struct {
	ID             string
	FullDatabaseID string
	Number         int
	Title          string
	State          string
	URL            string
	Body           string
	IsDraft        bool

	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  *time.Time
	MergedAt  *time.Time

	Repository *api.PRRepository
}

// Session is a hydrated in-flight agent task
type Session struct {
	ID              string
	Name            string
	UserID          int64
	AgentID         int64
	Logs            string
	State           string
	OwnerID         uint64
	RepoID          uint64
	ResourceType    string
	ResourceID      int64
	LastUpdatedAt   time.Time
	CreatedAt       time.Time
	CompletedAt     time.Time
	EventURL        string
	EventType       string
	PremiumRequests float64
	WorkflowRunID   uint64
	Error           *SessionError

	PullRequest *api.PullRequest
	User        *api.GitHubUser
}

type SessionError struct {
	Code    string
	Message string
}

type resource struct {
	ID                   string            `json:"id"`
	UserID               uint64            `json:"user_id"`
	ResourceType         string            `json:"resource_type"`
	ResourceID           int64             `json:"resource_id"`
	ResourceGlobalID     string            `json:"resource_global_id"`
	SessionCount         int               `json:"session_count"`
	SessionLastUpdatedAt int64             `json:"last_updated_at"`
	SessionState         string            `json:"state,omitempty"`
	ResourceState        string            `json:"resource_state"`
	Sessions             []resourceSession `json:"sessions"`
}

type resourceSession struct {
	SessionID            string `json:"id"`
	Name                 string `json:"name"`
	SessionState         string `json:"state,omitempty"`
	SessionLastUpdatedAt int64  `json:"last_updated_at"`
}

// ListLatestSessionsForViewer lists all agent sessions for the
// authenticated user up to limit.
func (c *CAPIClient) ListLatestSessionsForViewer(ctx context.Context, limit int) ([]*Session, error) {
	if limit == 0 {
		return nil, nil
	}

	url := baseCAPIURL + "/agents/sessions"
	pageSize := defaultSessionsPerPage

	seenResources := make(map[int64]struct{})
	latestSessions := make([]session, 0, limit)
	for page := 1; ; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return nil, err
		}

		q := req.URL.Query()
		q.Set("page_size", strconv.Itoa(pageSize))
		q.Set("page_number", strconv.Itoa(page))
		q.Set("sort", "last_updated_at,desc")
		req.URL.RawQuery = q.Encode()

		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to list sessions: %s", res.Status)
		}
		var response struct {
			Sessions []session `json:"sessions"`
		}
		if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode sessions response: %w", err)
		}

		// Process only the newly fetched page worth of sessions.
		pageSessions := response.Sessions

		// De-duplicate sessions by resource ID.
		// Because the API returns newest first, once we've seen
		// a resource ID we can ignore any older sessions for it.
		for _, s := range pageSessions {
			if _, exists := seenResources[s.ResourceID]; exists {
				continue
			}

			// A zero resource ID is a temporary situation before a PR/resource
			// is associated with the session. We should not mark such case as seen.
			if s.ResourceID != 0 {
				seenResources[s.ResourceID] = struct{}{}
			}

			latestSessions = append(latestSessions, s)
			if len(latestSessions) >= limit {
				break
			}
		}

		if len(response.Sessions) < pageSize || len(latestSessions) >= limit {
			break
		}
	}

	// Drop any above the limit
	if len(latestSessions) > limit {
		latestSessions = latestSessions[:limit]
	}

	result, err := c.hydrateSessionPullRequestsAndUsers(latestSessions)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session resources: %w", err)
	}

	return result, nil
}

// GetSession retrieves a specific agent session by ID.
func (c *CAPIClient) GetSession(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("missing session ID")
	}

	url := fmt.Sprintf("%s/agents/sessions/%s", baseCAPIURL, url.PathEscape(id))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusNotFound {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %s", res.Status)
	}

	var rawSession session
	if err := json.NewDecoder(res.Body).Decode(&rawSession); err != nil {
		return nil, fmt.Errorf("failed to decode session response: %w", err)
	}

	sessions, err := c.hydrateSessionPullRequestsAndUsers([]session{rawSession})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session resources: %w", err)
	}

	return sessions[0], nil
}

// GetSessionLogs retrieves logs of an agent session identified by ID.
func (c *CAPIClient) GetSessionLogs(ctx context.Context, id string) ([]byte, error) {
	if id == "" {
		return nil, fmt.Errorf("missing session ID")
	}

	url := fmt.Sprintf("%s/agents/sessions/%s/logs", baseCAPIURL, url.PathEscape(id))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusNotFound {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %s", res.Status)
	}

	return io.ReadAll(res.Body)
}

// ListSessionsByResourceID retrieves sessions associated with the given resource type and ID.
func (c *CAPIClient) ListSessionsByResourceID(ctx context.Context, resourceType string, resourceID int64, limit int) ([]*Session, error) {
	if resourceType == "" || resourceID == 0 {
		return nil, fmt.Errorf("missing resource type/ID")
	}

	if limit == 0 {
		return nil, nil
	}

	url := fmt.Sprintf("%s/agents/resource/%s/%d", baseCAPIURL, url.PathEscape(resourceType), resourceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list sessions: %s", res.Status)
	}

	var response resource
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode sessions response: %w", err)
	}

	sessions := make([]session, 0, len(response.Sessions))
	for _, s := range response.Sessions {
		session := session{
			ID:               s.SessionID,
			Name:             s.Name,
			UserID:           int64(response.UserID),
			ResourceType:     response.ResourceType,
			ResourceID:       response.ResourceID,
			ResourceGlobalID: response.ResourceGlobalID,
			State:            s.SessionState,
		}
		if s.SessionLastUpdatedAt != 0 {
			session.LastUpdatedAt = time.Unix(s.SessionLastUpdatedAt, 0).UTC()
		}
		sessions = append(sessions, session)
	}

	result, err := c.hydrateSessionPullRequestsAndUsers(sessions)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch session resources: %w", err)
	}
	return result, nil
}

// hydrateSessionPullRequestsAndUsers hydrates pull request and user information in sessions
func (c *CAPIClient) hydrateSessionPullRequestsAndUsers(sessions []session) ([]*Session, error) {
	if len(sessions) == 0 {
		return nil, nil
	}

	prNodeIds := make([]string, 0, len(sessions))
	userNodeIds := make([]string, 0, len(sessions))
	for _, session := range sessions {
		if session.ResourceType == "pull" {
			prNodeID := session.ResourceGlobalID
			// TODO: probably this can be dropped since the API should always
			// keep returning the resource global ID.
			if session.ResourceGlobalID == "" {
				prNodeID = generatePullRequestNodeID(int64(session.RepoID), session.ResourceID)
			}
			if !slices.Contains(prNodeIds, prNodeID) {
				prNodeIds = append(prNodeIds, prNodeID)
			}
		}

		userNodeId := generateUserNodeID(session.UserID)
		if !slices.Contains(userNodeIds, userNodeId) {
			userNodeIds = append(userNodeIds, userNodeId)
		}
	}
	apiClient := api.NewClientFromHTTP(c.httpClient)

	var resp struct {
		Nodes []struct {
			TypeName    string             `graphql:"__typename"`
			PullRequest sessionPullRequest `graphql:"... on PullRequest"`
			User        api.GitHubUser     `graphql:"... on User"`
		} `graphql:"nodes(ids: $ids)"`
	}

	ids := make([]string, 0, len(prNodeIds)+len(userNodeIds))
	ids = append(ids, prNodeIds...)
	ids = append(ids, userNodeIds...)

	// TODO handle pagination
	err := apiClient.Query(c.host, "FetchPRsAndUsersForAgentTaskSessions", &resp, map[string]any{
		"ids": ids,
	})

	if err != nil {
		return nil, err
	}

	prMap := make(map[string]*api.PullRequest, len(prNodeIds))
	userMap := make(map[int64]*api.GitHubUser, len(userNodeIds))
	for _, node := range resp.Nodes {
		switch node.TypeName {
		case "User":
			userMap[node.User.DatabaseID] = &node.User
		case "PullRequest":
			prMap[node.PullRequest.FullDatabaseID] = &api.PullRequest{
				ID:             node.PullRequest.ID,
				FullDatabaseID: node.PullRequest.FullDatabaseID,
				Number:         node.PullRequest.Number,
				Title:          node.PullRequest.Title,
				State:          node.PullRequest.State,
				IsDraft:        node.PullRequest.IsDraft,
				URL:            node.PullRequest.URL,
				Body:           node.PullRequest.Body,
				CreatedAt:      node.PullRequest.CreatedAt,
				UpdatedAt:      node.PullRequest.UpdatedAt,
				ClosedAt:       node.PullRequest.ClosedAt,
				MergedAt:       node.PullRequest.MergedAt,
				Repository:     node.PullRequest.Repository,
			}
		}
	}

	newSessions := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		newSession := fromAPISession(s)
		newSession.PullRequest = prMap[strconv.FormatInt(s.ResourceID, 10)]
		newSession.User = userMap[s.UserID]
		newSessions = append(newSessions, newSession)
	}

	return newSessions, nil
}

// GetPullRequestDatabaseID retrieves the database ID and URL of a pull request given its number in a repository.
func (c *CAPIClient) GetPullRequestDatabaseID(ctx context.Context, hostname string, owner string, repo string, number int) (int64, string, error) {
	// TODO: better int handling so we don't need to do bounds checks
	// to both ensure a panic is impossible and that we do not trigger
	// CodeQL alerts.
	if number <= 0 || number > math.MaxInt32 {
		return 0, "", fmt.Errorf("pull request number %d out of bounds", number)
	}

	var resp struct {
		Repository struct {
			PullRequest struct {
				FullDatabaseID string `graphql:"fullDatabaseId"`
				URL            string `graphql:"url"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"repo":   githubv4.String(repo),
		"number": githubv4.Int(number),
	}

	apiClient := api.NewClientFromHTTP(c.httpClient)
	if err := apiClient.Query(hostname, "GetPullRequestFullDatabaseID", &resp, variables); err != nil {
		return 0, "", err
	}

	databaseID, err := strconv.ParseInt(resp.Repository.PullRequest.FullDatabaseID, 10, 64)
	if err != nil {
		return 0, "", err
	}
	return databaseID, resp.Repository.PullRequest.URL, nil
}

// generatePullRequestNodeID converts an int64 databaseID and repoID to a GraphQL Node ID format
// with the "PR_" prefix for pull requests
func generatePullRequestNodeID(repoID, pullRequestID int64) string {
	buf := bytes.Buffer{}
	parts := []int64{0, repoID, pullRequestID}

	encoder := msgpack.NewEncoder(&buf)
	encoder.UseCompactInts(true)

	if err := encoder.Encode(parts); err != nil {
		panic(err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(buf.Bytes())

	return "PR_" + encoded
}

func generateUserNodeID(userID int64) string {
	buf := bytes.Buffer{}
	parts := []int64{0, userID}

	encoder := msgpack.NewEncoder(&buf)
	encoder.UseCompactInts(true)

	if err := encoder.Encode(parts); err != nil {
		panic(err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(buf.Bytes())

	return "U_" + encoded
}

func fromAPISession(s session) *Session {
	result := Session{
		ID:              s.ID,
		Name:            s.Name,
		UserID:          s.UserID,
		AgentID:         s.AgentID,
		Logs:            s.Logs,
		State:           s.State,
		OwnerID:         s.OwnerID,
		RepoID:          s.RepoID,
		ResourceType:    s.ResourceType,
		ResourceID:      s.ResourceID,
		LastUpdatedAt:   s.LastUpdatedAt,
		CreatedAt:       s.CreatedAt,
		CompletedAt:     s.CompletedAt,
		EventURL:        s.EventURL,
		EventType:       s.EventType,
		PremiumRequests: s.PremiumRequests,
		WorkflowRunID:   s.WorkflowRunID,
	}
	if s.Error != nil {
		result.Error = &SessionError{
			Code:    s.Error.Code,
			Message: s.Error.Message,
		}
	}
	return &result
}
