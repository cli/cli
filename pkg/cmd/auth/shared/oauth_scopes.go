package shared

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
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

// GetScopes performs a GitHub API request and returns the value of the
// X-Oauth-Scopes header.
//
// The token is passed per request rather than carried by the client, because
// these checks run against a token that is not yet, or no longer, the one the
// client would authenticate as: a token the user has just pasted, the previous
// token during a refresh, or each token in turn in gh auth status. The client
// is therefore expected to be an anonymous one.
func GetScopes(ctx context.Context, client *githubrest.Client, hostname, authToken string) (string, error) {
	apiEndpoint, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(hostname))
	if err != nil {
		return "", err
	}

	req, err := client.NewRequest(ctx, http.MethodGet, apiEndpoint.String(), nil, githubrest.WithSingleRequestToken(authToken))
	if err != nil {
		return "", err
	}

	// Send rather than Do, because the answer is in a response header. Do
	// closes the body; here it is drained first so the connection is reused
	// for the requests that follow.
	res, err := client.Send(req)
	if err != nil {
		if res != nil {
			res.Body.Close()
		}
		return "", err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()

	return res.Header.Get("X-Oauth-Scopes"), nil
}

// HasMinimumScopes performs a GitHub API request and returns an error if the token used in the request
// lacks the minimum required scopes for performing API operations with gh.
func HasMinimumScopes(ctx context.Context, client *githubrest.Client, hostname, authToken string) error {
	scopesHeader, err := GetScopes(ctx, client, hostname, authToken)
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
