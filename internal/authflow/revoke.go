package authflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
)

// RevokeToken revokes one access token issued by the GitHub CLI OAuth app.
func RevokeToken(httpClient *http.Client, hostname, token string) error {
	body, err := json.Marshal(struct {
		AccessToken string `json:"access_token"`
	}{AccessToken: token})
	if err != nil {
		return err
	}

	basicAuth := base64.StdEncoding.EncodeToString([]byte(oauthClientID + ":" + oauthClientSecret))
	resp, err := api.NewClientFromHTTP(httpClient).RequestWithContext(
		context.Background(), hostname, http.MethodDelete,
		fmt.Sprintf("applications/%s/token", oauthClientID), bytes.NewReader(body),
		api.WithHeader("Accept", "application/vnd.github+json"),
		api.WithHeader("Authorization", "Basic "+basicAuth),
		api.WithHeader("Content-Type", "application/json"),
		api.WithoutHTTPLogging())
	if err != nil {
		var httpErr api.HTTPError
		// The token may already be revoked or may have been issued by another OAuth app.
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return api.UnexpectedStatusError(resp)
	}

	return nil
}
