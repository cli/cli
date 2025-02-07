package api

// For descriptions of service interfaces, see:
// - https://docs.github.com/en/rest/reference/repos (for api.github.com)
// - https://github.com/github/github/blob/master/app/api/codespaces.rb (for vscs_internal)
// TODO(adonovan): replace the last link with a public doc URL when available.

// TODO(adonovan): a possible reorganization would be to split this
// file into two internal packages, one per backend service, and to
// rename api.API to github.Client:
//
// - github.GetUser(github.Client)
// - github.GetRepository(Client)
// - github.ReadFile(Client, nwo, branch, path) // was GetCodespaceRepositoryContents
// - codespaces.Create(Client, user, repo, sku, branch, location)
// - codespaces.Delete(Client, user, token, name)
// - codespaces.Get(Client, token, owner, name)
// - codespaces.GetMachineTypes(Client, user, repo, branch, location)
// - codespaces.GetToken(Client, login, name)
// - codespaces.List(Client, user)
// - codespaces.Start(Client, token, codespace)
//
// This would make the meaning of each operation clearer.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/opentracing/opentracing-go"
)

const (
	defaultAPIURL    = "https://api.github.com"
	defaultServerURL = "https://github.com"
)

const (
	VSCSTargetLocal       = "local"
	VSCSTargetDevelopment = "development"
	VSCSTargetPPE         = "ppe"
	VSCSTargetProduction  = "production"
)

// API is the interface to the codespace service.
type API struct {
	client       func() (*http.Client, error)
	githubAPI    string
	githubServer string
	retryBackoff time.Duration
}

// New creates a new API client connecting to the configured endpoints with the HTTP client.
func New(f *cmdutil.Factory) *API {
	apiURL := os.Getenv("GITHUB_API_URL")
	if apiURL == "" {
		cfg, err := f.Config()
		if err != nil {
			// fallback to the default api endpoint
			apiURL = defaultAPIURL
		} else {
			host, _ := cfg.Authentication().DefaultHost()
			apiURL = ghinstance.RESTPrefix(host)
		}
	}

	serverURL := os.Getenv("GITHUB_SERVER_URL")
	if serverURL == "" {
		cfg, err := f.Config()
		if err != nil {
			// fallback to the default server endpoint
			serverURL = defaultServerURL
		} else {
			host, _ := cfg.Authentication().DefaultHost()
			serverURL = ghinstance.HostPrefix(host)
		}
	}

	return &API{
		client:       f.HttpClient,
		githubAPI:    strings.TrimSuffix(apiURL, "/"),
		githubServer: strings.TrimSuffix(serverURL, "/"),
		retryBackoff: 100 * time.Millisecond,
	}
}

// User represents a GitHub user.
type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// ServerURL returns the server url (not the API url), such as https://github.com
func (a *API) ServerURL() string {
	return a.githubServer
}

// GetUser returns the user associated with the given token.
func (a *API) GetUser(ctx context.Context) (*User, error) {
	req, err := http.NewRequest(http.MethodGet, a.githubAPI+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/user")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response User
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return &response, nil
}

// RepositoryOwner represents owner of a repository
type RepositoryOwner struct {
	Type  string `json:"type"`
	Login string `json:"login"`
}

// Repository represents a GitHub repository.
type Repository struct {
	ID            int             `json:"id"`
	FullName      string          `json:"full_name"`
	DefaultBranch string          `json:"default_branch"`
	Owner         RepositoryOwner `json:"owner"`
}

// GetRepository returns the repository associated with the given owner and name.
func (a *API) GetRepository(ctx context.Context, nwo string) (*Repository, error) {
	req, err := http.NewRequest(http.MethodGet, a.githubAPI+"/repos/"+strings.ToLower(nwo), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/repos/*")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response Repository
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return &response, nil
}

// Codespace represents a codespace.
// You can see more about the fields in this type in the codespaces api docs:
// https://docs.github.com/en/rest/reference/codespaces
type Codespace struct {
	Name                           string              `json:"name"`
	CreatedAt                      string              `json:"created_at"`
	DisplayName                    string              `json:"display_name"`
	LastUsedAt                     string              `json:"last_used_at"`
	Owner                          User                `json:"owner"`
	Repository                     Repository          `json:"repository"`
	State                          string              `json:"state"`
	GitStatus                      CodespaceGitStatus  `json:"git_status"`
	Connection                     CodespaceConnection `json:"connection"`
	Machine                        CodespaceMachine    `json:"machine"`
	RuntimeConstraints             RuntimeConstraints  `json:"runtime_constraints"`
	VSCSTarget                     string              `json:"vscs_target"`
	PendingOperation               bool                `json:"pending_operation"`
	PendingOperationDisabledReason string              `json:"pending_operation_disabled_reason"`
	IdleTimeoutNotice              string              `json:"idle_timeout_notice"`
	WebURL                         string              `json:"web_url"`
	DevContainerPath               string              `json:"devcontainer_path"`
	Prebuild                       bool                `json:"prebuild"`
	Location                       string              `json:"location"`
	IdleTimeoutMinutes             int                 `json:"idle_timeout_minutes"`
	RetentionPeriodMinutes         int                 `json:"retention_period_minutes"`
	RetentionExpiresAt             string              `json:"retention_expires_at"`
	RecentFolders                  []string            `json:"recent_folders"`
	BillableOwner                  User                `json:"billable_owner"`
	EnvironmentId                  string              `json:"environment_id"`
}

type CodespaceGitStatus struct {
	Ahead                 int    `json:"ahead"`
	Behind                int    `json:"behind"`
	Ref                   string `json:"ref"`
	HasUnpushedChanges    bool   `json:"has_unpushed_changes"`
	HasUncommittedChanges bool   `json:"has_uncommitted_changes"`
}

type CodespaceMachine struct {
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	OperatingSystem string `json:"operating_system"`
	StorageInBytes  uint64 `json:"storage_in_bytes"`
	MemoryInBytes   uint64 `json:"memory_in_bytes"`
	CPUCount        int    `json:"cpus"`
}

const (
	// CodespaceStateAvailable is the state for a running codespace environment.
	CodespaceStateAvailable = "Available"
	// CodespaceStateShutdown is the state for a shutdown codespace environment.
	CodespaceStateShutdown = "Shutdown"
	// CodespaceStateShuttingDown is the state for a shutting down codespace environment.
	CodespaceStateShuttingDown = "ShuttingDown"
	// CodespaceStateStarting is the state for a starting codespace environment.
	CodespaceStateStarting = "Starting"
	// CodespaceStateRebuilding is the state for a rebuilding codespace environment.
	CodespaceStateRebuilding = "Rebuilding"
)

type CodespaceConnection struct {
	TunnelProperties TunnelProperties `json:"tunnelProperties"`
}

type TunnelProperties struct {
	ConnectAccessToken     string `json:"connectAccessToken"`
	ManagePortsAccessToken string `json:"managePortsAccessToken"`
	ServiceUri             string `json:"serviceUri"`
	TunnelId               string `json:"tunnelId"`
	ClusterId              string `json:"clusterId"`
	Domain                 string `json:"domain"`
}

type RuntimeConstraints struct {
	AllowedPortPrivacySettings []string `json:"allowed_port_privacy_settings"`
}

// ListCodespaceFields is the list of exportable fields for a codespace when using the `gh cs list` command.
var ListCodespaceFields = []string{
	"displayName",
	"name",
	"owner",
	"repository",
	"state",
	"gitStatus",
	"createdAt",
	"lastUsedAt",
	"machineName",
	"vscsTarget",
}

// ViewCodespaceFields is the list of exportable fields for a codespace when using the `gh cs view` command.
var ViewCodespaceFields = []string{
	"name",
	"displayName",
	"state",
	"owner",
	"billableOwner",
	"location",
	"repository",
	"gitStatus",
	"devcontainerPath",
	"machineName",
	"machineDisplayName",
	"prebuild",
	"createdAt",
	"lastUsedAt",
	"idleTimeoutMinutes",
	"retentionPeriodDays",
	"retentionExpiresAt",
	"recentFolders",
	"vscsTarget",
	"environmentId",
}

func (c *Codespace) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(c).Elem()
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		case "owner":
			data[f] = c.Owner.Login
		case "repository":
			data[f] = c.Repository.FullName
		case "machineName":
			data[f] = c.Machine.Name
		case "machineDisplayName":
			data[f] = c.Machine.DisplayName
		case "retentionPeriodDays":
			data[f] = c.RetentionPeriodMinutes / 1440
		case "gitStatus":
			data[f] = map[string]interface{}{
				"ref":                   c.GitStatus.Ref,
				"hasUnpushedChanges":    c.GitStatus.HasUnpushedChanges,
				"hasUncommittedChanges": c.GitStatus.HasUncommittedChanges,
				"ahead":                 c.GitStatus.Ahead,
				"behind":                c.GitStatus.Behind,
			}
		case "vscsTarget":
			if c.VSCSTarget != "" && c.VSCSTarget != VSCSTargetProduction {
				data[f] = c.VSCSTarget
			}
		default:
			sf := v.FieldByNameFunc(func(s string) bool {
				return strings.EqualFold(f, s)
			})
			data[f] = sf.Interface()
		}
	}

	return data
}

type ListCodespacesOptions struct {
	OrgName  string
	UserName string
	RepoName string
	Limit    int
}

// ListCodespaces returns a list of codespaces for the user. Pass a negative limit to request all pages from
// the API until all codespaces have been fetched.
func (a *API) ListCodespaces(ctx context.Context, opts ListCodespacesOptions) (codespaces []*Codespace, err error) {
	var (
		perPage = 100
		limit   = opts.Limit
	)

	if limit > 0 && limit < 100 {
		perPage = limit
	}

	var (
		listURL  string
		spanName string
	)

	if opts.RepoName != "" {
		listURL = fmt.Sprintf("%s/repos/%s/codespaces?per_page=%d", a.githubAPI, opts.RepoName, perPage)
		spanName = "/repos/*/codespaces"
	} else if opts.OrgName != "" {
		// the endpoints below can only be called by the organization admins
		orgName := opts.OrgName
		if opts.UserName != "" {
			userName := opts.UserName
			listURL = fmt.Sprintf("%s/orgs/%s/members/%s/codespaces?per_page=%d", a.githubAPI, orgName, userName, perPage)
			spanName = "/orgs/*/members/*/codespaces"
		} else {
			listURL = fmt.Sprintf("%s/orgs/%s/codespaces?per_page=%d", a.githubAPI, orgName, perPage)
			spanName = "/orgs/*/codespaces"
		}
	} else {
		listURL = fmt.Sprintf("%s/user/codespaces?per_page=%d", a.githubAPI, perPage)
		spanName = "/user/codespaces"
	}

	for {
		req, err := http.NewRequest(http.MethodGet, listURL, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		a.setHeaders(req)

		resp, err := a.do(ctx, req, spanName)
		if err != nil {
			return nil, fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, api.HandleHTTPError(resp)
		}

		var response struct {
			Codespaces []*Codespace `json:"codespaces"`
		}

		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&response); err != nil {
			return nil, fmt.Errorf("error unmarshalling response: %w", err)
		}

		nextURL := findNextPage(resp.Header.Get("Link"))
		codespaces = append(codespaces, response.Codespaces...)

		if nextURL == "" || (limit > 0 && len(codespaces) >= limit) {
			break
		}

		if newPerPage := limit - len(codespaces); limit > 0 && newPerPage < 100 {
			u, _ := url.Parse(nextURL)
			q := u.Query()
			q.Set("per_page", strconv.Itoa(newPerPage))
			u.RawQuery = q.Encode()
			listURL = u.String()
		} else {
			listURL = nextURL
		}
	}

	return codespaces, nil
}

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

func findNextPage(linkValue string) string {
	for _, m := range linkRE.FindAllStringSubmatch(linkValue, -1) {
		if len(m) > 2 && m[2] == "next" {
			return m[1]
		}
	}
	return ""
}

func (a *API) GetOrgMemberCodespace(ctx context.Context, orgName string, userName string, codespaceName string) (*Codespace, error) {
	perPage := 100
	listURL := fmt.Sprintf("%s/orgs/%s/members/%s/codespaces?per_page=%d", a.githubAPI, orgName, userName, perPage)

	for {
		req, err := http.NewRequest(http.MethodGet, listURL, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		a.setHeaders(req)

		resp, err := a.do(ctx, req, "/orgs/*/members/*/codespaces")
		if err != nil {
			return nil, fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, api.HandleHTTPError(resp)
		}

		var response struct {
			Codespaces []*Codespace `json:"codespaces"`
		}

		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&response); err != nil {
			return nil, fmt.Errorf("error unmarshalling response: %w", err)
		}

		for _, cs := range response.Codespaces {
			if cs.Name == codespaceName {
				return cs, nil
			}
		}

		nextURL := findNextPage(resp.Header.Get("Link"))
		if nextURL == "" {
			break
		}
		listURL = nextURL
	}

	return nil, fmt.Errorf("codespace not found for user %s with name %s", userName, codespaceName)
}

// GetCodespace returns the user codespace based on the provided name.
// If the codespace is not found, an error is returned.
// If includeConnection is true, it will return the connection information for the codespace.
func (a *API) GetCodespace(ctx context.Context, codespaceName string, includeConnection bool) (*Codespace, error) {
	resp, err := a.withRetry(func() (*http.Response, error) {
		req, err := http.NewRequest(
			http.MethodGet,
			a.githubAPI+"/user/codespaces/"+codespaceName,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		if includeConnection {
			q := req.URL.Query()
			q.Add("internal", "true")
			q.Add("refresh", "true")
			req.URL.RawQuery = q.Encode()
		}
		a.setHeaders(req)
		return a.do(ctx, req, "/user/codespaces/*")
	})
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return &response, nil
}

// StartCodespace starts a codespace for the user.
// If the codespace is already running, the returned error from the API is ignored.
func (a *API) StartCodespace(ctx context.Context, codespaceName string) error {
	resp, err := a.withRetry(func() (*http.Response, error) {
		req, err := http.NewRequest(
			http.MethodPost,
			a.githubAPI+"/user/codespaces/"+codespaceName+"/start",
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		a.setHeaders(req)
		return a.do(ctx, req, "/user/codespaces/*/start")
	})
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusConflict {
			// 409 means the codespace is already running which we can safely ignore
			return nil
		}
		return api.HandleHTTPError(resp)
	}

	return nil
}

func (a *API) StopCodespace(ctx context.Context, codespaceName string, orgName string, userName string) error {
	var stopURL string
	var spanName string

	if orgName != "" {
		stopURL = fmt.Sprintf("%s/orgs/%s/members/%s/codespaces/%s/stop", a.githubAPI, orgName, userName, codespaceName)
		spanName = "/orgs/*/members/*/codespaces/*/stop"
	} else {
		stopURL = fmt.Sprintf("%s/user/codespaces/%s/stop", a.githubAPI, codespaceName)
		spanName = "/user/codespaces/*/stop"
	}

	req, err := http.NewRequest(http.MethodPost, stopURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, spanName)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return api.HandleHTTPError(resp)
	}

	return nil
}

type Machine struct {
	Name                 string `json:"name"`
	DisplayName          string `json:"display_name"`
	PrebuildAvailability string `json:"prebuild_availability"`
}

// GetCodespacesMachines returns the codespaces machines for the given repo, branch and location.
func (a *API) GetCodespacesMachines(ctx context.Context, repoID int, branch, location string, devcontainerPath string) ([]*Machine, error) {
	reqURL := fmt.Sprintf("%s/repositories/%d/codespaces/machines", a.githubAPI, repoID)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("location", location)
	q.Add("ref", branch)
	q.Add("devcontainer_path", devcontainerPath)
	req.URL.RawQuery = q.Encode()

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/repositories/*/codespaces/machines")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response struct {
		Machines []*Machine `json:"machines"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return response.Machines, nil
}

// GetCodespacesPermissionsCheck returns a bool indicating whether the user has accepted permissions for the given repo and devcontainer path.
func (a *API) GetCodespacesPermissionsCheck(ctx context.Context, repoID int, branch string, devcontainerPath string) (bool, error) {
	reqURL := fmt.Sprintf("%s/repositories/%d/codespaces/permissions_check", a.githubAPI, repoID)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("error creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("ref", branch)
	q.Add("devcontainer_path", devcontainerPath)
	req.URL.RawQuery = q.Encode()

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/repositories/*/codespaces/permissions_check")
	if err != nil {
		return false, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("error reading response body: %w", err)
	}

	var response struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return false, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return response.Accepted, nil
}

// RepoSearchParameters are the optional parameters for searching for repositories.
type RepoSearchParameters struct {
	// The maximum number of repos to return. At most 100 repos are returned even if this value is greater than 100.
	MaxRepos int
	// The sort order for returned repos. Possible values are 'stars', 'forks', 'help-wanted-issues', or 'updated'. If empty the API's default ordering is used.
	Sort string
}

// GetCodespaceRepoSuggestions searches for and returns repo names based on the provided search text.
func (a *API) GetCodespaceRepoSuggestions(ctx context.Context, partialSearch string, parameters RepoSearchParameters) ([]string, error) {
	reqURL := fmt.Sprintf("%s/search/repositories", a.githubAPI)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	parts := strings.SplitN(partialSearch, "/", 2)

	var nameSearch string
	if len(parts) == 2 {
		user := parts[0]
		repo := parts[1]
		nameSearch = fmt.Sprintf("%s user:%s", repo, user)
	} else {
		/*
		 * This results in searching for the text within the owner or the name. It's possible to
		 * do an owner search and then look up some repos for those owners, but that adds a
		 * good amount of latency to the fetch which slows down showing the suggestions.
		 */
		nameSearch = partialSearch
	}

	queryStr := fmt.Sprintf("%s in:name", nameSearch)

	q := req.URL.Query()
	q.Add("q", queryStr)

	if len(parameters.Sort) > 0 {
		q.Add("sort", parameters.Sort)
	}

	if parameters.MaxRepos > 0 {
		q.Add("per_page", strconv.Itoa(parameters.MaxRepos))
	}

	req.URL.RawQuery = q.Encode()

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/search/repositories/*")
	if err != nil {
		return nil, fmt.Errorf("error searching repositories: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response struct {
		Items []*Repository `json:"items"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	repoNames := make([]string, len(response.Items))
	for i, repo := range response.Items {
		repoNames[i] = repo.FullName
	}

	return repoNames, nil
}

// GetCodespaceBillableOwner returns the billable owner and expected default values for
// codespaces created by the user for a given repository.
func (a *API) GetCodespaceBillableOwner(ctx context.Context, nwo string) (*User, error) {
	req, err := http.NewRequest(http.MethodGet, a.githubAPI+"/repos/"+nwo+"/codespaces/new", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/repos/*/codespaces/new")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	} else if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("you cannot create codespaces with that repository")
	} else if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response struct {
		BillableOwner User `json:"billable_owner"`
		Defaults      struct {
			DevcontainerPath string `json:"devcontainer_path"`
			Location         string `json:"location"`
		}
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	// While this response contains further helpful information ahead of codespace creation,
	// we're only referencing the billable owner today.
	return &response.BillableOwner, nil
}

// CreateCodespaceParams are the required parameters for provisioning a Codespace.
type CreateCodespaceParams struct {
	RepositoryID           int
	IdleTimeoutMinutes     int
	RetentionPeriodMinutes *int
	Branch                 string
	Machine                string
	Location               string
	DevContainerPath       string
	VSCSTarget             string
	VSCSTargetURL          string
	PermissionsOptOut      bool
	DisplayName            string
}

// CreateCodespace creates a codespace with the given parameters and returns a non-nil error if it
// fails to create.
func (a *API) CreateCodespace(ctx context.Context, params *CreateCodespaceParams) (*Codespace, error) {
	codespace, err := a.startCreate(ctx, params)
	if !errors.Is(err, errProvisioningInProgress) {
		return codespace, err
	}

	// errProvisioningInProgress indicates that codespace creation did not complete
	// within the GitHub API RPC time limit (10s), so it continues asynchronously.
	// We must poll the server to discover the outcome.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			codespace, err = a.GetCodespace(ctx, codespace.Name, false)
			if err != nil {
				return nil, fmt.Errorf("failed to get codespace: %w", err)
			}

			// we continue to poll until the codespace shows as provisioned
			if codespace.State != CodespaceStateAvailable {
				continue
			}

			return codespace, nil
		}
	}
}

type startCreateRequest struct {
	RepositoryID           int    `json:"repository_id"`
	IdleTimeoutMinutes     int    `json:"idle_timeout_minutes,omitempty"`
	RetentionPeriodMinutes *int   `json:"retention_period_minutes,omitempty"`
	Ref                    string `json:"ref"`
	Location               string `json:"location"`
	Machine                string `json:"machine"`
	DevContainerPath       string `json:"devcontainer_path,omitempty"`
	VSCSTarget             string `json:"vscs_target,omitempty"`
	VSCSTargetURL          string `json:"vscs_target_url,omitempty"`
	PermissionsOptOut      bool   `json:"multi_repo_permissions_opt_out"`
	DisplayName            string `json:"display_name"`
}

var errProvisioningInProgress = errors.New("provisioning in progress")

type AcceptPermissionsRequiredError struct {
	Message             string `json:"message"`
	AllowPermissionsURL string `json:"allow_permissions_url"`
}

func (e AcceptPermissionsRequiredError) Error() string {
	return e.Message
}

// startCreate starts the creation of a codespace.
// It may return success or an error, or errProvisioningInProgress indicating that the operation
// did not complete before the GitHub API's time limit for RPCs (10s), in which case the caller
// must poll the server to learn the outcome.
func (a *API) startCreate(ctx context.Context, params *CreateCodespaceParams) (*Codespace, error) {
	if params == nil {
		return nil, errors.New("startCreate missing parameters")
	}

	requestBody, err := json.Marshal(startCreateRequest{
		RepositoryID:           params.RepositoryID,
		IdleTimeoutMinutes:     params.IdleTimeoutMinutes,
		RetentionPeriodMinutes: params.RetentionPeriodMinutes,
		Ref:                    params.Branch,
		Location:               params.Location,
		Machine:                params.Machine,
		DevContainerPath:       params.DevContainerPath,
		VSCSTarget:             params.VSCSTarget,
		VSCSTargetURL:          params.VSCSTargetURL,
		PermissionsOptOut:      params.PermissionsOptOut,
		DisplayName:            params.DisplayName,
	})

	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, a.githubAPI+"/user/codespaces", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/user/codespaces")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}

		var response Codespace
		if err := json.Unmarshal(b, &response); err != nil {
			return nil, fmt.Errorf("error unmarshalling response: %w", err)
		}

		return &response, errProvisioningInProgress // RPC finished before result of creation known
	} else if resp.StatusCode == http.StatusUnauthorized {
		var (
			ue       AcceptPermissionsRequiredError
			bodyCopy = &bytes.Buffer{}
			r        = io.TeeReader(resp.Body, bodyCopy)
		)

		b, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}
		if err := json.Unmarshal(b, &ue); err != nil {
			return nil, fmt.Errorf("error unmarshalling response: %w", err)
		}

		if ue.AllowPermissionsURL != "" {
			return nil, ue
		}

		resp.Body = io.NopCloser(bodyCopy)

		return nil, api.HandleHTTPError(resp)

	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return &response, nil
}

// DeleteCodespace deletes the given codespace.
func (a *API) DeleteCodespace(ctx context.Context, codespaceName string, orgName string, userName string) error {
	var deleteURL string
	var spanName string

	if orgName != "" && userName != "" {
		deleteURL = fmt.Sprintf("%s/orgs/%s/members/%s/codespaces/%s", a.githubAPI, orgName, userName, codespaceName)
		spanName = "/orgs/*/members/*/codespaces/*"
	} else {
		deleteURL = a.githubAPI + "/user/codespaces/" + codespaceName
		spanName = "/user/codespaces/*"
	}

	req, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, spanName)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return api.HandleHTTPError(resp)
	}

	return nil
}

type DevContainerEntry struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

// ListDevContainers returns a list of valid devcontainer.json files for the repo. Pass a negative limit to request all pages from
// the API until all devcontainer.json files have been fetched.
func (a *API) ListDevContainers(ctx context.Context, repoID int, branch string, limit int) (devcontainers []DevContainerEntry, err error) {
	perPage := 100
	if limit > 0 && limit < 100 {
		perPage = limit
	}

	v := url.Values{}
	v.Set("per_page", strconv.Itoa(perPage))
	if branch != "" {
		v.Set("ref", branch)
	}
	listURL := fmt.Sprintf("%s/repositories/%d/codespaces/devcontainers?%s", a.githubAPI, repoID, v.Encode())

	for {
		req, err := http.NewRequest(http.MethodGet, listURL, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		a.setHeaders(req)

		resp, err := a.do(ctx, req, fmt.Sprintf("/repositories/%d/codespaces/devcontainers", repoID))
		if err != nil {
			return nil, fmt.Errorf("error making request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, api.HandleHTTPError(resp)
		}

		var response struct {
			Devcontainers []DevContainerEntry `json:"devcontainers"`
		}

		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(&response); err != nil {
			return nil, fmt.Errorf("error unmarshalling response: %w", err)
		}

		nextURL := findNextPage(resp.Header.Get("Link"))
		devcontainers = append(devcontainers, response.Devcontainers...)

		if nextURL == "" || (limit > 0 && len(devcontainers) >= limit) {
			break
		}

		if newPerPage := limit - len(devcontainers); limit > 0 && newPerPage < 100 {
			u, _ := url.Parse(nextURL)
			q := u.Query()
			q.Set("per_page", strconv.Itoa(newPerPage))
			u.RawQuery = q.Encode()
			listURL = u.String()
		} else {
			listURL = nextURL
		}
	}

	return devcontainers, nil
}

type EditCodespaceParams struct {
	DisplayName        string `json:"display_name,omitempty"`
	IdleTimeoutMinutes int    `json:"idle_timeout_minutes,omitempty"`
	Machine            string `json:"machine,omitempty"`
}

func (a *API) EditCodespace(ctx context.Context, codespaceName string, params *EditCodespaceParams) (*Codespace, error) {
	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, a.githubAPI+"/user/codespaces/"+codespaceName, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/user/codespaces/*")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 422 (unprocessable entity) is likely caused by the codespace having a
		// pending op, so we'll fetch the codespace to see if that's the case
		// and return a more understandable error message.
		if resp.StatusCode == http.StatusUnprocessableEntity {
			pendingOp, reason, err := a.checkForPendingOperation(ctx, codespaceName)
			// If there's an error or there's not a pending op, we want to let
			// this fall through to the normal api.HandleHTTPError flow
			if err == nil && pendingOp {
				return nil, fmt.Errorf(
					"codespace is disabled while it has a pending operation: %s",
					reason,
				)
			}
		}
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	return &response, nil
}

func (a *API) checkForPendingOperation(ctx context.Context, codespaceName string) (bool, string, error) {
	codespace, err := a.GetCodespace(ctx, codespaceName, false)
	if err != nil {
		return false, "", err
	}
	return codespace.PendingOperation, codespace.PendingOperationDisabledReason, nil
}

type getCodespaceRepositoryContentsResponse struct {
	Content string `json:"content"`
}

func (a *API) GetCodespaceRepositoryContents(ctx context.Context, codespace *Codespace, path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, a.githubAPI+"/repos/"+codespace.Repository.FullName+"/contents/"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("ref", codespace.GitStatus.Ref)
	req.URL.RawQuery = q.Encode()

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/repos/*/contents/*")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	} else if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response getCodespaceRepositoryContentsResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(response.Content)
	if err != nil {
		return nil, fmt.Errorf("error decoding content: %w", err)
	}

	return decoded, nil
}

// do executes the given request and returns the response. It creates an
// opentracing span to track the length of the request.
func (a *API) do(ctx context.Context, req *http.Request, spanName string) (*http.Response, error) {
	// TODO(adonovan): use NewRequestWithContext(ctx) and drop ctx parameter.
	span, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	defer span.Finish()
	req = req.WithContext(ctx)

	httpClient, err := a.client()
	if err != nil {
		return nil, err
	}

	return httpClient.Do(req)
}

// setHeaders sets the required headers for the API.
func (a *API) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}

// withRetry takes a generic function that sends an http request and retries
// only when the returned response has a >=500 status code.
func (a *API) withRetry(f func() (*http.Response, error)) (*http.Response, error) {
	bo := backoff.NewConstantBackOff(a.retryBackoff)
	return backoff.RetryWithData(func() (*http.Response, error) {
		resp, err := f()
		if err != nil {
			return nil, backoff.Permanent(err)
		}
		if resp.StatusCode < 500 {
			return resp, nil
		}
		return nil, fmt.Errorf("received response with status code %d", resp.StatusCode)
	}, backoff.WithMaxRetries(bo, 3))
}

// HTTPClient returns the HTTP client used to make requests to the API.
func (a *API) HTTPClient() (*http.Client, error) {
	httpClient, err := a.client()
	if err != nil {
		return nil, err
	}

	return httpClient, nil
}
