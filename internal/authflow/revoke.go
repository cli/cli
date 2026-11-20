package authflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/safeurl"
)

// ErrTokenNotFound reports a 404 from targeted token revocation.
var ErrTokenNotFound = errors.New("HTTP 404 (Not Found)")

// RevokeToken deletes one access token belonging to the GitHub CLI OAuth app.
// The client must not attach user authentication or log credential-bearing bodies.
func RevokeToken(ctx context.Context, client *http.Client, hostname, token string) error {
	if token == "" {
		return errors.New("authentication token is empty")
	}
	body, err := json.Marshal(struct {
		AccessToken string `json:"access_token"`
	}{AccessToken: token})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	basicAuth := base64.StdEncoding.EncodeToString([]byte(oauthClientID + ":" + oauthClientSecret))
	path, err := safeurl.JoinPath("applications", oauthClientID, "token")
	if err != nil {
		return err
	}
	resp, err := api.NewClientFromHTTP(client).RequestWithContext(
		ctx, hostname, http.MethodDelete, path.String(), bytes.NewReader(body),
		api.WithHeader("Authorization", "Basic "+basicAuth),
		api.WithHeader("Accept", "application/vnd.github+json"),
		api.WithHeader("Content-Type", "application/json"),
		api.WithoutFollowingRedirects(),
	)
	if err != nil {
		if httpErr, ok := errors.AsType[api.HTTPError](err); ok {
			// A server error message may echo the access token supplied in the body.
			if httpErr.StatusCode == http.StatusNotFound {
				return ErrTokenNotFound
			}
			return fmt.Errorf("HTTP %d (%s)", httpErr.StatusCode, http.StatusText(httpErr.StatusCode))
		}
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
			return errors.New("request timed out")
		}
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		return errors.New(strings.NewReplacer(
			token, "[REDACTED]",
			oauthClientSecret, "[REDACTED]",
			basicAuth, "[REDACTED]",
		).Replace(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return api.UnexpectedStatusError(resp)
	}
	return nil
}
