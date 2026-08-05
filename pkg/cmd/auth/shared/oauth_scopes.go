package shared

import (
	"context"
	"fmt"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"
	"strings"

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

// GetScopes performs a GitHub API request and returns the value of the X-Oauth-Scopes header.
func GetScopes(httpClient *http.Client, hostname, authToken string) (string, error) {
	// The token being checked is not necessarily the configured one, so it is
	// supplied per request rather than at client construction. WithoutToken
	// then guarantees no other Authorization can reach the wire.
	client, err := githubrest.NewClient(githubrest.APIBaseURL(hostname), httpClient, githubrest.WithoutToken())
	if err != nil {
		return "", err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, "", nil,
		githubrest.WithSingleRequestToken(authToken))
	if err != nil {
		return "", err
	}

	// Do drains and closes the body, which this relies on so that the
	// connection is reused for the request that follows.
	res, err := client.Do(req, nil)
	if err != nil {
		return "", err
	}

	return res.Header.Get("X-Oauth-Scopes"), nil
}

// HasMinimumScopes performs a GitHub API request and returns an error if the token used in the request
// lacks the minimum required scopes for performing API operations with gh.
func HasMinimumScopes(httpClient *http.Client, hostname, authToken string) error {
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
