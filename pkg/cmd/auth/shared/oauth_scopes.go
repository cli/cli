package shared

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
)

type MissingScopesError struct {
	MissingScopes []string
}

func (e MissingScopesError) Error() string {
	var missing []string
	for _, s := range e.MissingScopes {
		missing = append(missing, fmt.Sprintf("'%s'", s))
	}
	scopes := strings.Join(missing, ", ")

	if len(e.MissingScopes) == 1 {
		return "missing required scope " + scopes
	}
	return "missing required scopes " + scopes
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

// GetScopes performs a GitHub API request and returns the value of the X-Oauth-Scopes header.
func GetScopes(httpClient httpClient, hostname, authToken string) (string, error) {
	apiEndpoint := ghinstance.RESTPrefix(hostname)

	req, err := http.NewRequest("GET", apiEndpoint, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "token "+authToken)

	res, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}

	defer func() {
		// Ensure the response body is fully read and closed
		// before we reconnect, so that we reuse the same TCPconnection.
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()

	if res.StatusCode != 200 {
		return "", api.HandleHTTPError(res)
	}

	return res.Header.Get("X-Oauth-Scopes"), nil
}

// HasMinimumScopes performs a GitHub API request and returns an error if the token used in the request
// lacks the minimum required scopes for performing API operations with gh.
func HasMinimumScopes(httpClient httpClient, hostname, authToken string) error {
	scopesHeader, err := GetScopes(httpClient, hostname, authToken)
	if err != nil {
		return err
	}

	return HeaderHasMinimumScopes(scopesHeader)
}

// HeaderHasMinimumScopes parses the comma separated scopesHeader string and returns an error
// if it lacks the minimum required scopes for performing API operations with gh.
func HeaderHasMinimumScopes(scopesHeader string) error {
	if scopesHeader == "" {
		// if the token reports no scopes, assume that it's an integration token and give up on
		// detecting its capabilities
		return nil
	}

	search := map[string]bool{
		"repo":      false,
		"read:org":  false,
		"admin:org": false,
	}
	for _, s := range strings.Split(scopesHeader, ",") {
		search[strings.TrimSpace(s)] = true
	}

	var missingScopes []string
	if !search["repo"] {
		missingScopes = append(missingScopes, "repo")
	}

	if !search["read:org"] && !search["write:org"] && !search["admin:org"] {
		missingScopes = append(missingScopes, "read:org")
	}

	if len(missingScopes) > 0 {
		return &MissingScopesError{MissingScopes: missingScopes}
	}
	return nil
}
