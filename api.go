package liveshare

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type api struct {
	client      *Client
	httpClient  *http.Client
	serviceURI  string
	workspaceID string
}

func newAPI(client *Client) *api {
	serviceURI := client.liveShare.Configuration.LiveShareEndpoint
	if !strings.HasSuffix(client.liveShare.Configuration.LiveShareEndpoint, "/") {
		serviceURI = client.liveShare.Configuration.LiveShareEndpoint + "/"
	}

	if !strings.Contains(serviceURI, "api/v1.2") {
		serviceURI = serviceURI + "api/v1.2"
	}

	serviceURI = strings.TrimSuffix(serviceURI, "/")

	return &api{client, &http.Client{}, serviceURI, strings.ToUpper(client.liveShare.Configuration.WorkspaceID)}
}

type workspaceAccessResponse struct {
	SessionToken              string            `json:"sessionToken"`
	CreatedAt                 string            `json:"createdAt"`
	UpdatedAt                 string            `json:"updatedAt"`
	Name                      string            `json:"name"`
	OwnerID                   string            `json:"ownerId"`
	JoinLink                  string            `json:"joinLink"`
	ConnectLinks              []string          `json:"connectLinks"`
	RelayLink                 string            `json:"relayLink"`
	RelaySas                  string            `json:"relaySas"`
	HostPublicKeys            []string          `json:"hostPublicKeys"`
	ConversationID            string            `json:"conversationId"`
	AssociatedUserIDs         map[string]string `json:"associatedUserIds"`
	AreAnonymousGuestsAllowed bool              `json:"areAnonymousGuestsAllowed"`
	IsHostConnected           bool              `json:"isHostConnected"`
	ExpiresAt                 string            `json:"expiresAt"`
	InvitationLinks           []string          `json:"invitationLinks"`
	ID                        string            `json:"id"`
}

func (a *api) workspaceAccess() (*workspaceAccessResponse, error) {
	url := fmt.Sprintf("%s/workspace/%s/user", a.serviceURI, a.workspaceID)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setDefaultHeaders(req)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	var response workspaceAccessResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response into json: %v", err)
	}

	return &response, nil
}

func (a *api) setDefaultHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.client.liveShare.Configuration.Token)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/json")
}

type workspaceInfoResponse struct {
	CreatedAt                 string   `json:"createdAt"`
	UpdatedAt                 string   `json:"updatedAt"`
	Name                      string   `json:"name"`
	OwnerID                   string   `json:"ownerId"`
	JoinLink                  string   `json:"joinLink"`
	ConnectLinks              []string `json:"connectLinks"`
	RelayLink                 string   `json:"relayLink"`
	RelaySas                  string   `json:"relaySas"`
	HostPublicKeys            []string `json:"hostPublicKeys"`
	ConversationID            string   `json:"conversationId"`
	AssociatedUserIDs         map[string]string
	AreAnonymousGuestsAllowed bool     `json:"areAnonymousGuestsAllowed"`
	IsHostConnected           bool     `json:"isHostConnected"`
	ExpiresAt                 string   `json:"expiresAt"`
	InvitationLinks           []string `json:"invitationLinks"`
	ID                        string   `json:"id"`
}

func (a *api) workspaceInfo() (*workspaceInfoResponse, error) {
	url := fmt.Sprintf("%s/workspace/%s", a.serviceURI, a.workspaceID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setDefaultHeaders(req)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	var response workspaceInfoResponse
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, fmt.Errorf("error unmarshaling response into json: %v", err)
	}

	return &response, nil
}
