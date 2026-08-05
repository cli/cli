package githubrest

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

const (
	acceptedScopesHeader = "X-Accepted-Oauth-Scopes"
	tokenScopesHeader    = "X-Oauth-Scopes"
)

// ErrorItem stores additional information about an error response returned
// from the GitHub REST API. It mirrors go-gh's HTTPErrorItem.
type ErrorItem struct {
	Code     string
	Field    string
	Message  string
	Resource string
}

// ErrorResponse is a non-2xx response from the GitHub REST API, destructured
// into its content. It is only ever constructed when the API successfully
// responded and reported failure in its own payload, so a transport-level
// failure such as a DNS error or a connection reset is never this type.
type ErrorResponse struct {
	Errors     []ErrorItem
	Headers    http.Header
	Message    string
	RequestURL *url.URL
	StatusCode int

	// requiredScopes are scopes the endpoint needs but did not report, as
	// declared by RequireScopes.
	requiredScopes []string
}

// Error allows ErrorResponse to satisfy the error interface.
func (e *ErrorResponse) Error() string {
	if msgs := strings.SplitN(e.Message, "\n", 2); len(msgs) > 1 {
		return fmt.Sprintf("HTTP %d: %s (%s)\n%s", e.StatusCode, msgs[0], e.RequestURL, msgs[1])
	} else if e.Message != "" {
		return fmt.Sprintf("HTTP %d: %s (%s)", e.StatusCode, e.Message, e.RequestURL)
	}
	return fmt.Sprintf("HTTP %d (%s)", e.StatusCode, e.RequestURL)
}

// ScopesSuggestion returns a suggestion to request additional OAuth scopes when
// the response indicates that the token is missing a scope the endpoint needs,
// and "" when there is nothing to suggest.
//
// It is computed from the fields of the error rather than stored, so a
// suggestion is always available when one exists. It is safe to call on a nil
// or zero-valued receiver, which matters because callers reach it holding a
// nil pointer after a failed errors.As.
func (e *ErrorResponse) ScopesSuggestion() string {
	if e == nil {
		return ""
	}

	var hostname string
	if e.RequestURL != nil {
		hostname = e.RequestURL.Hostname()
	}

	accepted := e.Headers.Get(acceptedScopesHeader)
	if len(e.requiredScopes) > 0 {
		accepted = strings.TrimPrefix(accepted+", "+strings.Join(e.requiredScopes, ", "), ", ")
	}

	return generateScopesSuggestion(
		e.StatusCode,
		accepted,
		e.Headers.Get(tokenScopesHeader),
		hostname,
	)
}

// RequireScopes declares scopes the endpoint needs but does not report in
// X-Accepted-Oauth-Scopes, and returns the receiver so it can be used inline.
//
// Some endpoints simply do not list the scopes they require, so a 4xx from them
// produces no suggestion at all and the user is told only that they lack
// access. The CLI's existing answer, api.EndpointNeedsScopes, forges the header
// on the response as though the server had sent it. Recording it on the error
// instead keeps the response an honest record of what arrived, and puts the
// claim where the thing that consumes it already lives.
//
// It is safe to call on a nil receiver, because callers reach it holding a nil
// pointer after a failed errors.As.
func (e *ErrorResponse) RequireScopes(scopes ...string) *ErrorResponse {
	if e == nil {
		return nil
	}
	e.requiredScopes = append(e.requiredScopes, scopes...)
	return e
}

// NewErrorResponse parses a non-2xx response into an *ErrorResponse.
//
// It exists for callers that already hold a response and need the error for it,
// which is what api.HandleHTTPError does today. Send and Do build their own, so
// most callers never need this.
//
// It reads the response body but does not close it, matching go-gh's
// HandleHTTPError, which does the payload parsing.
//
// resp.Request may be nil, which happens with a custom RoundTripper because
// net/http only populates Response.Request for its own transport. go-gh's
// HandleHTTPError dereferences resp.Request.URL unguarded, so parsing runs
// against a shallow copy carrying a placeholder request rather than against the
// caller's response, which is left untouched. RequestURL then stays nil rather
// than becoming an empty *url.URL, because an empty URL satisfies every
// dereference and then lies in error messages.
func NewErrorResponse(resp *http.Response) *ErrorResponse {
	errResp := &ErrorResponse{
		Headers:    resp.Header,
		StatusCode: resp.StatusCode,
	}

	parseFrom := resp
	if resp.Request == nil {
		// http.Response holds no locks, so copying it is safe.
		copied := *resp
		copied.Request = &http.Request{}
		parseFrom = &copied
	} else {
		errResp.RequestURL = resp.Request.URL
	}

	// HandleHTTPError always returns a *ghAPI.HTTPError, but fall back rather
	// than type assert so a change upstream cannot turn into a panic here.
	ghErr, ok := ghAPI.HandleHTTPError(parseFrom).(*ghAPI.HTTPError)
	if !ok {
		errResp.Message = resp.Status
		return errResp
	}

	errResp.Message = ghErr.Message
	for _, item := range ghErr.Errors {
		errResp.Errors = append(errResp.Errors, ErrorItem(item))
	}
	return errResp
}

func generateScopesSuggestion(statusCode int, endpointNeedsScopes, tokenHasScopes, hostname string) string {
	if statusCode < 400 || statusCode > 499 || statusCode == 422 {
		return ""
	}

	if tokenHasScopes == "" {
		return ""
	}

	gotScopes := map[string]struct{}{}
	for _, s := range strings.Split(tokenHasScopes, ",") {
		s = strings.TrimSpace(s)
		gotScopes[s] = struct{}{}

		// Certain scopes may be grouped under a single "top-level" scope. The following branch
		// statements include these grouped/implied scopes when the top-level scope is encountered.
		// See https://docs.github.com/en/developers/apps/building-oauth-apps/scopes-for-oauth-apps.
		if s == "repo" {
			gotScopes["repo:status"] = struct{}{}
			gotScopes["repo_deployment"] = struct{}{}
			gotScopes["public_repo"] = struct{}{}
			gotScopes["repo:invite"] = struct{}{}
			gotScopes["security_events"] = struct{}{}
		} else if s == "user" {
			gotScopes["read:user"] = struct{}{}
			gotScopes["user:email"] = struct{}{}
			gotScopes["user:follow"] = struct{}{}
		} else if s == "codespace" {
			gotScopes["codespace:secrets"] = struct{}{}
		} else if strings.HasPrefix(s, "admin:") {
			gotScopes["read:"+strings.TrimPrefix(s, "admin:")] = struct{}{}
			gotScopes["write:"+strings.TrimPrefix(s, "admin:")] = struct{}{}
		} else if strings.HasPrefix(s, "write:") {
			gotScopes["read:"+strings.TrimPrefix(s, "write:")] = struct{}{}
		}
	}

	for _, s := range strings.Split(endpointNeedsScopes, ",") {
		s = strings.TrimSpace(s)
		if _, gotScope := gotScopes[s]; s == "" || gotScope {
			continue
		}
		return fmt.Sprintf(
			"This API operation needs the %[1]q scope. To request it, run:  gh auth refresh -h %[2]s -s %[1]s",
			s,
			ghauth.NormalizeHostname(hostname),
		)
	}

	return ""
}
