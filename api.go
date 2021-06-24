package liveshare

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type API struct {
	Configuration *Configuration
	HttpClient    *http.Client
	ServiceURI    string
	WorkspaceID   string
}

func NewAPI(configuration *Configuration) *API {
	serviceURI := configuration.LiveShareEndpoint
	if !strings.HasSuffix(configuration.LiveShareEndpoint, "/") {
		serviceURI = configuration.LiveShareEndpoint + "/"
	}

	if !strings.Contains(serviceURI, "api/v1.2") {
		serviceURI = serviceURI + "api/v1.2"
	}

	serviceURI = strings.TrimSuffix(serviceURI, "/")

	return &API{configuration, &http.Client{}, serviceURI, strings.ToUpper(configuration.WorkspaceID)}
}

type WorkspaceAccessResponse struct {
	SessionToken              string   `json:"sessionToken"`
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
	AssociatedUserIDs         []string `json:"associatedUserIds"`
	AreAnonymousGuestsAllowed bool     `json:"areAnonymousGuestsAllowed"`
	IsHostConnected           bool     `json:"isHostConnected"`
	ExpiresAt                 string   `json:"expiresAt"`
	InvitationLinks           []string `json:"invitationLinks"`
	ID                        string   `json:"id"`
}

func (a *API) WorkspaceAccess() (*WorkspaceAccessResponse, error) {
	url := fmt.Sprintf("%s/workspace/%s/user", a.ServiceURI, a.WorkspaceID)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setDefaultHeaders(req)
	resp, err := a.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	var workspaceAccessResponse WorkspaceAccessResponse
	if err := json.Unmarshal(b, &workspaceAccessResponse); err != nil {
		return nil, fmt.Errorf("error unmarshaling response into json: %v", err)
	}

	return &workspaceAccessResponse, nil
}

func (a *API) setDefaultHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.Configuration.Token)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Content-Type", "application/json")
}

type WorkspaceInfoResponse struct {
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
	AssociatedUserIDs         []string `json:"associatedUserIds"`
	AreAnonymousGuestsAllowed bool     `json:"areAnonymousGuestsAllowed"`
	IsHostConnected           bool     `json:"isHostConnected"`
	ExpiresAt                 string   `json:"expiresAt"`
	InvitationLinks           []string `json:"invitationLinks"`
	ID                        string   `json:"id"`
}

func (a *API) WorkspaceInfo() (*WorkspaceInfoResponse, error) {
	url := fmt.Sprintf("%s/workspace/%s", a.ServiceURI, a.WorkspaceID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	a.setDefaultHeaders(req)
	resp, err := a.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %v", err)
	}

	var workspaceInfoResponse WorkspaceInfoResponse
	if err := json.Unmarshal(b, &workspaceInfoResponse); err != nil {
		return nil, fmt.Errorf("error unmarshaling response into json: %v", err)
	}

	return &workspaceInfoResponse, nil
}
