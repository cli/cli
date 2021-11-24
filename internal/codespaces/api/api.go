package api

// For descriptions of service interfaces, see:
// - https://online.visualstudio.com/api/swagger (for visualstudio.com)
// - https://docs.github.com/en/rest/reference/repos (for api.github.com)
// - https://github.com/github/github/blob/master/app/api/codespaces.rb (for vscs_internal)
// TODO(adonovan): replace the last link with a public doc URL when available.

// TODO(adonovan): a possible reorganization would be to split this
// file into three internal packages, one per backend service, and to
// rename api.API to github.Client:
//
// - github.GetUser(github.Client)
// - github.GetRepository(Client)
// - github.ReadFile(Client, nwo, branch, path) // was GetCodespaceRepositoryContents
// - github.AuthorizedKeys(Client, user)
// - codespaces.Create(Client, user, repo, sku, branch, location)
// - codespaces.Delete(Client, user, token, name)
// - codespaces.Get(Client, token, owner, name)
// - codespaces.GetMachineTypes(Client, user, repo, branch, location)
// - codespaces.GetToken(Client, login, name)
// - codespaces.List(Client, user)
// - codespaces.Start(Client, token, codespace)
// - visualstudio.GetRegionLocation(http.Client) // no dependency on github
//
// This would make the meaning of each operation clearer.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/opentracing/opentracing-go"
)

const (
	githubServer = "https://github.com"
	githubAPI    = "https://api.github.com"
	vscsAPI      = "https://online.visualstudio.com"
)

// API is the interface to the codespace service.
type API struct {
	client       httpClient
	vscsAPI      string
	githubAPI    string
	githubServer string
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a new API client connecting to the configured endpoints with the HTTP client.
func New(serverURL, apiURL, vscsURL string, httpClient httpClient) *API {
	if serverURL == "" {
		serverURL = githubServer
	}
	if apiURL == "" {
		apiURL = githubAPI
	}
	if vscsURL == "" {
		vscsURL = vscsAPI
	}
	return &API{
		client:       httpClient,
		vscsAPI:      strings.TrimSuffix(vscsURL, "/"),
		githubAPI:    strings.TrimSuffix(apiURL, "/"),
		githubServer: strings.TrimSuffix(serverURL, "/"),
	}
}

// User represents a GitHub user.
type User struct {
	Login string `json:"login"`
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response User
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &response, nil
}

// Repository represents a GitHub repository.
type Repository struct {
	ID            int    `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response Repository
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &response, nil
}

// Codespace represents a codespace.
type Codespace struct {
	Name       string              `json:"name"`
	CreatedAt  string              `json:"created_at"`
	LastUsedAt string              `json:"last_used_at"`
	Owner      User                `json:"owner"`
	Repository Repository          `json:"repository"`
	State      string              `json:"state"`
	GitStatus  CodespaceGitStatus  `json:"git_status"`
	Connection CodespaceConnection `json:"connection"`
}

type CodespaceGitStatus struct {
	Ahead                int    `json:"ahead"`
	Behind               int    `json:"behind"`
	Ref                  string `json:"ref"`
	HasUnpushedChanges   bool   `json:"has_unpushed_changes"`
	HasUncommitedChanges bool   `json:"has_uncommited_changes"`
}

const (
	// CodespaceStateAvailable is the state for a running codespace environment.
	CodespaceStateAvailable = "Available"
	// CodespaceStateShutdown is the state for a shutdown codespace environment.
	CodespaceStateShutdown = "Shutdown"
	// CodespaceStateStarting is the state for a starting codespace environment.
	CodespaceStateStarting = "Starting"
)

type CodespaceConnection struct {
	SessionID      string   `json:"sessionId"`
	SessionToken   string   `json:"sessionToken"`
	RelayEndpoint  string   `json:"relayEndpoint"`
	RelaySAS       string   `json:"relaySas"`
	HostPublicKeys []string `json:"hostPublicKeys"`
}

// CodespaceFields is the list of exportable fields for a codespace.
var CodespaceFields = []string{
	"name",
	"owner",
	"repository",
	"state",
	"gitStatus",
	"createdAt",
	"lastUsedAt",
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
		case "gitStatus":
			data[f] = map[string]interface{}{
				"ref":                  c.GitStatus.Ref,
				"hasUnpushedChanges":   c.GitStatus.HasUnpushedChanges,
				"hasUncommitedChanges": c.GitStatus.HasUncommitedChanges,
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

// ListCodespaces returns a list of codespaces for the user. Pass a negative limit to request all pages from
// the API until all codespaces have been fetched.
func (a *API) ListCodespaces(ctx context.Context, limit int) (codespaces []*Codespace, err error) {
	perPage := 100
	if limit > 0 && limit < 100 {
		perPage = limit
	}

	listURL := fmt.Sprintf("%s/user/codespaces?per_page=%d", a.githubAPI, perPage)
	for {
		req, err := http.NewRequest(http.MethodGet, listURL, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		a.setHeaders(req)

		resp, err := a.do(ctx, req, "/user/codespaces")
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
			return nil, fmt.Errorf("error unmarshaling response: %w", err)
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

// GetCodespace returns the user codespace based on the provided name.
// If the codespace is not found, an error is returned.
// If includeConnection is true, it will return the connection information for the codespace.
func (a *API) GetCodespace(ctx context.Context, codespaceName string, includeConnection bool) (*Codespace, error) {
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
	resp, err := a.do(ctx, req, "/user/codespaces/*")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &response, nil
}

// StartCodespace starts a codespace for the user.
// If the codespace is already running, the returned error from the API is ignored.
func (a *API) StartCodespace(ctx context.Context, codespaceName string) error {
	req, err := http.NewRequest(
		http.MethodPost,
		a.githubAPI+"/user/codespaces/"+codespaceName+"/start",
		nil,
	)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/user/codespaces/*/start")
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

func (a *API) StopCodespace(ctx context.Context, codespaceName string) error {
	req, err := http.NewRequest(
		http.MethodPost,
		a.githubAPI+"/user/codespaces/"+codespaceName+"/stop",
		nil,
	)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/user/codespaces/*/stop")
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return api.HandleHTTPError(resp)
	}

	return nil
}

type getCodespaceRegionLocationResponse struct {
	Current string `json:"current"`
}

// GetCodespaceRegionLocation returns the closest codespace location for the user.
func (a *API) GetCodespaceRegionLocation(ctx context.Context) (string, error) {
	req, err := http.NewRequest(http.MethodGet, a.vscsAPI+"/api/v1/locations", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	resp, err := a.do(ctx, req, req.URL.String())
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	var response getCodespaceRegionLocationResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	return response.Current, nil
}

type Machine struct {
	Name                 string `json:"name"`
	DisplayName          string `json:"display_name"`
	PrebuildAvailability string `json:"prebuild_availability"`
}

// GetCodespacesMachines returns the codespaces machines for the given repo, branch and location.
func (a *API) GetCodespacesMachines(ctx context.Context, repoID int, branch, location string) ([]*Machine, error) {
	reqURL := fmt.Sprintf("%s/repositories/%d/codespaces/machines", a.githubAPI, repoID)
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("location", location)
	q.Add("ref", branch)
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response struct {
		Machines []*Machine `json:"machines"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return response.Machines, nil
}

// CreateCodespaceParams are the required parameters for provisioning a Codespace.
type CreateCodespaceParams struct {
	RepositoryID       int
	IdleTimeoutMinutes int
	Branch             string
	Machine            string
	Location           string
}

// CreateCodespace creates a codespace with the given parameters and returns a non-nil error if it
// fails to create.
func (a *API) CreateCodespace(ctx context.Context, params *CreateCodespaceParams) (*Codespace, error) {
	codespace, err := a.startCreate(ctx, params)
	if err != errProvisioningInProgress {
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
	RepositoryID       int    `json:"repository_id"`
	IdleTimeoutMinutes int    `json:"idle_timeout_minutes,omitempty"`
	Ref                string `json:"ref"`
	Location           string `json:"location"`
	Machine            string `json:"machine"`
}

var errProvisioningInProgress = errors.New("provisioning in progress")

// startCreate starts the creation of a codespace.
// It may return success or an error, or errProvisioningInProgress indicating that the operation
// did not complete before the GitHub API's time limit for RPCs (10s), in which case the caller
// must poll the server to learn the outcome.
func (a *API) startCreate(ctx context.Context, params *CreateCodespaceParams) (*Codespace, error) {
	if params == nil {
		return nil, errors.New("startCreate missing parameters")
	}

	requestBody, err := json.Marshal(startCreateRequest{
		RepositoryID:       params.RepositoryID,
		IdleTimeoutMinutes: params.IdleTimeoutMinutes,
		Ref:                params.Branch,
		Location:           params.Location,
		Machine:            params.Machine,
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
		return nil, errProvisioningInProgress // RPC finished before result of creation known
	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &response, nil
}

// DeleteCodespace deletes the given codespace.
func (a *API) DeleteCodespace(ctx context.Context, codespaceName string) error {
	req, err := http.NewRequest(http.MethodDelete, a.githubAPI+"/user/codespaces/"+codespaceName, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/user/codespaces/*")
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return api.HandleHTTPError(resp)
	}

	return nil
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	var response getCodespaceRepositoryContentsResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(response.Content)
	if err != nil {
		return nil, fmt.Errorf("error decoding content: %w", err)
	}

	return decoded, nil
}

// AuthorizedKeys returns the public keys (in ~/.ssh/authorized_keys
// format) registered by the specified GitHub user.
func (a *API) AuthorizedKeys(ctx context.Context, user string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s.keys", a.githubServer, user)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.do(ctx, req, "/user.keys")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}
	return b, nil
}

// do executes the given request and returns the response. It creates an
// opentracing span to track the length of the request.
func (a *API) do(ctx context.Context, req *http.Request, spanName string) (*http.Response, error) {
	// TODO(adonovan): use NewRequestWithContext(ctx) and drop ctx parameter.
	span, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	defer span.Finish()
	req = req.WithContext(ctx)
	return a.client.Do(req)
}

// setHeaders sets the required headers for the API.
func (a *API) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}
