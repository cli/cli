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

	"github.com/opentracing/opentracing-go"
)

const githubAPI = "https://api.github.com"

type API struct {
	token  string
	client *http.Client
}

func New(token string) *API {
	return &API{token, &http.Client{}}
}

type User struct {
	Login string `json:"login"`
}

func (a *API) GetUser(ctx context.Context) (*User, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/user", nil)
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

func jsonErrorResponse(b []byte) error {
	var response struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return fmt.Errorf("error unmarshaling error response: %w", err)
	}

	return errors.New(response.Message)
}

type Repository struct {
	ID int `json:"id"`
}

func (a *API) GetRepository(ctx context.Context, nwo string) (*Repository, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/repos/"+strings.ToLower(nwo), nil)
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

type Codespace struct {
	Name           string               `json:"name"`
	GUID           string               `json:"guid"`
	CreatedAt      string               `json:"created_at"`
	Branch         string               `json:"branch"`
	RepositoryName string               `json:"repository_name"`
	RepositoryNWO  string               `json:"repository_nwo"`
	OwnerLogin     string               `json:"owner_login"`
	Environment    CodespaceEnvironment `json:"environment"`
}

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
	CodespaceEnvironmentStateAvailable = "Available"
)

type CodespaceEnvironmentConnection struct {
	SessionID     string `json:"sessionId"`
	SessionToken  string `json:"sessionToken"`
	RelayEndpoint string `json:"relayEndpoint"`
	RelaySAS      string `json:"relaySas"`
}

func (a *API) ListCodespaces(ctx context.Context, user *User) ([]*Codespace, error) {
	req, err := http.NewRequest(
		http.MethodGet, githubAPI+"/vscs_internal/user/"+user.Login+"/codespaces", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/vscs_internal/user/*/codespaces")
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
		Codespaces []*Codespace `json:"codespaces"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}
	return response.Codespaces, nil
}

type getCodespaceTokenRequest struct {
	MintRepositoryToken bool `json:"mint_repository_token"`
}

type getCodespaceTokenResponse struct {
	RepositoryToken string `json:"repository_token"`
}

func (a *API) GetCodespaceToken(ctx context.Context, ownerLogin, codespaceName string) (string, error) {
	reqBody, err := json.Marshal(getCodespaceTokenRequest{true})
	if err != nil {
		return "", fmt.Errorf("error preparing request body: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		githubAPI+"/vscs_internal/user/"+ownerLogin+"/codespaces/"+codespaceName+"/token",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/vscs_internal/user/*/codespaces/*/token")
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

	var response getCodespaceTokenResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %w", err)
	}

	return response.RepositoryToken, nil
}

func (a *API) GetCodespace(ctx context.Context, token, owner, codespace string) (*Codespace, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		githubAPI+"/vscs_internal/user/"+owner+"/codespaces/"+codespace,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.do(ctx, req, "/vscs_internal/user/*/codespaces/*")
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

func (a *API) StartCodespace(ctx context.Context, token string, codespace *Codespace) error {
	req, err := http.NewRequest(
		http.MethodPost,
		githubAPI+"/vscs_internal/proxy/environments/"+codespace.GUID+"/start",
		nil,
	)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.do(ctx, req, "/vscs_internal/proxy/environments/*/start")
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Error response is typically a numeric code (not an error message, nor JSON).
		if len(b) > 100 {
			b = append(b[:97], "..."...)
		}
		if strings.TrimSpace(string(b)) == "7" {
			// Non-HTTP 200 with error code 7 (EnvironmentNotShutdown) is benign.
			// Ignore it.
		} else {
			return fmt.Errorf("failed to start codespace: %s", b)
		}
	}

	return nil
}

type getCodespaceRegionLocationResponse struct {
	Current string `json:"current"`
}

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

type SKU struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

func (a *API) GetCodespacesSKUs(ctx context.Context, user *User, repository *Repository, branch, location string) ([]*SKU, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/vscs_internal/user/"+user.Login+"/skus", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	q := req.URL.Query()
	q.Add("location", location)
	q.Add("ref", branch)
	q.Add("repository_id", strconv.Itoa(repository.ID))
	req.URL.RawQuery = q.Encode()

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/vscs_internal/user/*/skus")
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
		SKUs []*SKU `json:"skus"`
	}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return response.SKUs, nil
}

type createCodespaceRequest struct {
	RepositoryID int    `json:"repository_id"`
	Ref          string `json:"ref"`
	Location     string `json:"location"`
	SkuName      string `json:"sku_name"`
}

func (a *API) CreateCodespace(ctx context.Context, user *User, repository *Repository, sku, branch, location string) (*Codespace, error) {
	requestBody, err := json.Marshal(createCodespaceRequest{repository.ID, branch, location, sku})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, githubAPI+"/vscs_internal/user/"+user.Login+"/codespaces", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	a.setHeaders(req)
	resp, err := a.do(ctx, req, "/vscs_internal/user/*/codespaces")
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	if resp.StatusCode > http.StatusAccepted {
		return nil, jsonErrorResponse(b)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return &response, nil
}

func (a *API) DeleteCodespace(ctx context.Context, user *User, token, codespaceName string) error {
	req, err := http.NewRequest(http.MethodDelete, githubAPI+"/vscs_internal/user/"+user.Login+"/codespaces/"+codespaceName, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.do(ctx, req, "/vscs_internal/user/*/codespaces/*")
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
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/repos/"+codespace.RepositoryNWO+"/contents/"+path, nil)
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

func (a *API) do(ctx context.Context, req *http.Request, spanName string) (*http.Response, error) {
	// TODO(adonovan): use NewRequestWithContext(ctx) and drop ctx parameter.
	span, ctx := opentracing.StartSpanFromContext(ctx, spanName)
	defer span.Finish()
	req = req.WithContext(ctx)
	return a.client.Do(req)
}

func (a *API) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}
