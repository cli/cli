package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

type errResponse struct {
	Message string `json:"message"`
}

func (a *API) GetUser(ctx context.Context) (*User, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setHeaders(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, a.errorResponse(b)
	}

	var response User
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return &response, nil
}

func (a *API) errorResponse(b []byte) error {
	var response errResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return fmt.Errorf("error unmarshaling error response: %v", err)
	}

	return errors.New(response.Message)
}

type Repository struct {
	ID int `json:"id"`
}

func (a *API) GetRepository(ctx context.Context, nwo string) (*Repository, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/repos/"+strings.ToLower(nwo), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setHeaders(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, a.errorResponse(b)
	}

	var response Repository
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return &response, nil
}

type Codespaces []*Codespace

func (c Codespaces) SortByRecent() {
	sort.Slice(c, func(i, j int) bool {
		return c[i].CreatedAt > c[j].CreatedAt
	})
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
}

const (
	CodespaceEnvironmentStateAvailable = "Available"
)

type CodespaceEnvironmentConnection struct {
	SessionID    string `json:"sessionId"`
	SessionToken string `json:"sessionToken"`
}

func (a *API) ListCodespaces(ctx context.Context, user *User) (Codespaces, error) {
	req, err := http.NewRequest(
		http.MethodGet, githubAPI+"/vscs_internal/user/"+user.Login+"/codespaces", nil,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setHeaders(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, a.errorResponse(b)
	}

	response := struct {
		Codespaces Codespaces `json:"codespaces"`
	}{}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}
	return response.Codespaces, nil
}

type getCodespaceTokenRequest struct {
	MintRepositoryToken bool `json:"mint_repository_token"`
}

type getCodespaceTokenResponse struct {
	RepositoryToken string `json:"repository_token"`
}

func (a *API) GetCodespaceToken(ctx context.Context, codespace *Codespace) (string, error) {
	reqBody, err := json.Marshal(getCodespaceTokenRequest{true})
	if err != nil {
		return "", fmt.Errorf("error preparing request body: %v", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		githubAPI+"/vscs_internal/user/"+codespace.OwnerLogin+"/codespaces/"+codespace.Name+"/token",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	a.setHeaders(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", a.errorResponse(b)
	}

	var response getCodespaceTokenResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
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
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, a.errorResponse(b)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
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
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	_, err = a.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}

	return nil
}

type getCodespaceRegionLocationResponse struct {
	Current string `json:"current"`
}

func (a *API) GetCodespaceRegionLocation(ctx context.Context) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://online.visualstudio.com/api/v1/locations", nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}

	var response getCodespaceRegionLocationResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return "", fmt.Errorf("error unmarshaling response: %v", err)
	}

	return response.Current, nil
}

type Skus []*Sku

type Sku struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

func (a *API) GetCodespacesSkus(ctx context.Context, user *User, repository *Repository, location string) (Skus, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI+"/vscs_internal/user/"+user.Login+"/skus", nil)
	if err != nil {
		return nil, fmt.Errorf("err creating request: %v", err)
	}

	q := req.URL.Query()
	q.Add("location", location)
	q.Add("repository_id", strconv.Itoa(repository.ID))
	req.URL.RawQuery = q.Encode()

	a.setHeaders(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	response := struct {
		Skus Skus `json:"skus"`
	}{}
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return response.Skus, nil
}

type createCodespaceRequest struct {
	RepositoryID int    `json:"repository_id"`
	Ref          string `json:"ref"`
	Location     string `json:"location"`
	SkuName      string `json:"sku_name"`
}

func (a *API) CreateCodespace(ctx context.Context, user *User, repository *Repository, sku *Sku, branch, location string) (*Codespace, error) {
	requestBody, err := json.Marshal(createCodespaceRequest{repository.ID, branch, location, sku.Name})
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, githubAPI+"/vscs_internal/user/"+user.Login+"/codespaces", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setHeaders(req)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	if resp.StatusCode > http.StatusAccepted {
		return nil, a.errorResponse(b)
	}

	var response Codespace
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %v", err)
	}

	return &response, nil
}

func (a *API) DeleteCodespace(ctx context.Context, user *User, token, codespaceName string) error {
	req, err := http.NewRequest(http.MethodDelete, githubAPI+"/vscs_internal/user/"+user.Login+"/codespaces/"+codespaceName, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode > http.StatusAccepted {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %v", err)
		}
		return a.errorResponse(b)
	}

	return nil
}

func (a *API) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}
