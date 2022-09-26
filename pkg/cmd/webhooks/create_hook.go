package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cli/cli/v2/api"
)

type createHookRequest struct {
	Name   string     `json:"name"`
	Events []string   `json:"events"`
	Active bool       `json:"active"`
	Config hookConfig `json:"config"`
}

type hookConfig struct {
	ContentType string `json:"content_type"`
	InsecureSSL string `json:"insecure_ssl"`
	URL         string `json:"url"`
}

type createHookResponse struct {
	Active bool       `json:"active"`
	Config hookConfig `json:"config"`
	Events []string   `json:"events"`
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	URL    string     `json:"url"`
	WsURL  string     `json:"ws_url"`
}

// createHook issues a request against the GitHub API to create a dev webhook
func createHook(o *hookOptions) (string, error) {
	httpClient, err := o.HttpClient()
	if err != nil {
		return "", err
	}
	apiClient := api.NewClientFromHTTP(httpClient)
	path := fmt.Sprintf("repos/%s/hooks", o.Repo)
	req := createHookRequest{
		Name:   "cli",
		Events: o.EventTypes,
		Active: true,
		Config: hookConfig{
			ContentType: "json",
			InsecureSSL: "0",
		},
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	var res createHookResponse
	err = apiClient.REST(o.Host, "POST", path, bytes.NewReader(reqBytes), &res)
	if err != nil {
		return "", err
	}
	return res.WsURL, nil
}
