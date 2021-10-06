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
	"strconv"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
)

const githubAPI = "https://api.github.com"

// API is the interface to the codespace service.
type API struct {
	token     string
	client    httpClient
	githubAPI string
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// New creates a new API client with the given token and HTTP client.
func New(token string, httpClient httpClient) *API {
	return &API{
		token:     token,
		client:    httpClient,
		githubAPI: githubAPI,
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, jsonErrorResponse(b)
	}

	var response User
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &response, nil
}

// jsonErrorResponse returns the error message from a JSON response.
func jsonErrorResponse(b []byte) error {
	var response struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return fmt.Errorf("error unmarshaling error response: %w", err)
	}

	return errors.New(response.Message)
}

// Repository represents a GitHub repository.
type Repository struct {
	ID int `json:"id"`
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, jsonErrorResponse(b)
	}

	var response Repository
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &response, nil
}

// Codespace represents a codespace.
type Codespace struct {
	Name           string               `json:"name"`
	CreatedAt      string               `json:"created_at"`
	LastUsedAt     string               `json:"last_used_at"`
	State          string               `json:"state"`
	Branch         string               `json:"branch"`
	RepositoryName string               `json:"repository_name"`
	RepositoryNWO  string               `json:"repository_nwo"`
	OwnerLogin     string               `json:"owner_login"`
	Environment    CodespaceEnvironment `json:"environment"`
}

const CodespaceStateProvisioned = "provisioned"

type CodespaceEnvironment struct {
	State      string                         `json:"state"`
	Connection CodespaceEnvironmentConnection `json:"connection"`
	GitStatus  CodespaceEnvironmentGitStatus  `json:"gitStatus"`
}

type CodespaceEnvironmentGitStatus struct {
	Ahead                int    `json:"ahead"`
	Behind               int    `json:"behind"`
	Branch               string `json:"branch"`
	Commit               string `json:"commit"`
	HasUnpushedChanges   bool   `json:"hasUnpushedChanges"`
	HasUncommitedChanges bool   `json:"hasUncommitedChanges"`
}

const (
	// CodespaceEnvironmentStateAvailable is the state for a running codespace environment.
	CodespaceEnvironmentStateAvailable = "Available"
)

type CodespaceEnvironmentConnection struct {
	SessionID      string   `json:"sessionId"`
	SessionToken   string   `json:"sessionToken"`
	RelayEndpoint  string   `json:"relayEndpoint"`
	RelaySAS       string   `json:"relaySas"`
	HostPublicKeys []string `json:"hostPublicKeys"`
}

// codespacesListResponse is the response body for the `/user/codespaces` endpoint
type getCodespacesListResponse struct {
	Codespaces []*Codespace `json:"codespaces"`
	TotalCount int          `json:"total_count"`
}

// ListCodespaces returns a list of codespaces for the user.
// It consumes all pages returned by the API until all codespaces have been fetched.
func (a *API) ListCodespaces(ctx context.Context) (codespaces []*Codespace, err error) {
	per_page := 100
	for page := 1; ; page++ {
		response, err := a.fetchCodespaces(ctx, page, per_page)
		if err != nil {
			return nil, err
		}
		codespaces = append(codespaces, response.Codespaces...)
		if page*per_page >= response.TotalCount {
			break
		}
	}

	return codespaces, nil
}

func (a *API) fetchCodespaces(ctx context.Context, page int, per_page int) (response *getCodespacesListResponse, err error) {
	req, err := http.NewRequest(
		http.MethodGet, a.githubAPI+"/user/codespaces", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	a.setHeaders(req)
	q := req.URL.Query()
	q.Add("page", strconv.Itoa(page))
	q.Add("per_page", strconv.Itoa(per_page))

	req.URL.RawQuery = q.Encode()
	resp, err := a.do(ctx, req, "/user/codespaces")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, jsonErrorResponse(b)
	}

	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}
	return response, nil
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, jsonErrorResponse(b)
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusConflict {
			// 409 means the codespace is already running which we can safely ignore
			return nil
		}

		// Error response may be a numeric code or a JSON {"message": "..."}.
		if bytes.HasPrefix(b, []byte("{")) {
			return jsonErrorResponse(b) // probably JSON
		}

		if len(b) > 100 {
			b = append(b[:97], "..."...)
		}
		return fmt.Errorf("failed to start codespace: %s", b)
	}

	return nil
}

type getCodespaceRegionLocationResponse struct {
	Current string `json:"current"`
}

// GetCodespaceRegionLocation returns the closest codespace location for the user.
func (a *API) GetCodespaceRegionLocation(ctx context.Context) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://online.visualstudio.com/api/v1/locations", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	resp, err := a.do(ctx, req, req.URL.String())
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", jsonErrorResponse(b)
	}

	var response getCodespaceRegionLocationResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	return response.Current, nil
}

type Machine struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, jsonErrorResponse(b)
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
	RepositoryID              int
	Branch, Machine, Location string
}

// CreateCodespace creates a codespace with the given parameters and returns a non-nil error if it
// fails to create.
func (a *API) CreateCodespace(ctx context.Context, params *CreateCodespaceParams) (*Codespace, error) {
	codespace, err := a.startCreate(ctx, params.RepositoryID, params.Machine, params.Branch, params.Location)
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
			if codespace.State != CodespaceStateProvisioned {
				continue
			}

			return codespace, nil
		}
	}
}

type startCreateRequest struct {
	RepositoryID int    `json:"repository_id"`
	Ref          string `json:"ref"`
	Location     string `json:"location"`
	Machine      string `json:"machine"`
}

var errProvisioningInProgress = errors.New("provisioning in progress")

// startCreate starts the creation of a codespace.
// It may return success or an error, or errProvisioningInProgress indicating that the operation
// did not complete before the GitHub API's time limit for RPCs (10s), in which case the caller
// must poll the server to learn the outcome.
func (a *API) startCreate(ctx context.Context, repoID int, machine, branch, location string) (*Codespace, error) {
	requestBody, err := json.Marshal(startCreateRequest{repoID, branch, location, machine})
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

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	switch {
	case resp.StatusCode > http.StatusAccepted:
		return nil, jsonErrorResponse(b)
	case resp.StatusCode == http.StatusAccepted:
		return nil, errProvisioningInProgress // RPC finished before result of creation known
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

	if resp.StatusCode > http.StatusAccepted {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		return jsonErrorResponse(b)
	}

	return nil
}

type getCodespaceRepositoryContentsResponse struct {
	Content string `json:"content"`
}

func (a *API) GetCodespaceRepositoryContents(ctx context.Context, codespace *Codespace, path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, a.githubAPI+"/repos/"+codespace.RepositoryNWO+"/contents/"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("ref", codespace.Branch)
	req.URL.RawQuery = q.Encode()

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/repos/*/contents/*")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, jsonErrorResponse(b)
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
	url := fmt.Sprintf("https://github.com/%s.keys", user)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.do(ctx, req, "/user.keys")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
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
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}
