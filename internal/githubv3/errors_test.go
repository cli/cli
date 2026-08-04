package githubv3

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorResponseError(t *testing.T) {
	requestURL, err := url.Parse("https://api.github.com/repos/OWNER/REPO")
	require.NoError(t, err)

	tests := []struct {
		name    string
		err     *ErrorResponse
		wantMsg string
	}{
		{
			name: "message",
			err: &ErrorResponse{
				Message:    "Not Found",
				RequestURL: requestURL,
				StatusCode: 404,
			},
			wantMsg: "HTTP 404: Not Found (https://api.github.com/repos/OWNER/REPO)",
		},
		{
			name: "multi-line message",
			err: &ErrorResponse{
				Message:    "Validation Failed\nIssue.title is missing",
				RequestURL: requestURL,
				StatusCode: 422,
			},
			wantMsg: "HTTP 422: Validation Failed (https://api.github.com/repos/OWNER/REPO)\nIssue.title is missing",
		},
		{
			name: "no message",
			err: &ErrorResponse{
				RequestURL: requestURL,
				StatusCode: 500,
			},
			wantMsg: "HTTP 500 (https://api.github.com/repos/OWNER/REPO)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantMsg, tt.err.Error())
		})
	}
}

// TestScopesSuggestionNilSafety pins the property a later migration step
// depends on: internal/ghcmd reaches ScopesSuggestion holding a nil pointer
// when errors.As returned false, and must not panic there.
func TestScopesSuggestionNilSafety(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var err *ErrorResponse
		assert.NotPanics(t, func() {
			assert.Equal(t, "", err.ScopesSuggestion())
		})
	})

	t.Run("zero value", func(t *testing.T) {
		err := &ErrorResponse{}
		assert.NotPanics(t, func() {
			assert.Equal(t, "", err.ScopesSuggestion())
		})
	})

	t.Run("nil RequestURL with scopes present", func(t *testing.T) {
		err := &ErrorResponse{
			StatusCode: 404,
			Headers: http.Header{
				"X-Accepted-Oauth-Scopes": []string{"read:org"},
				"X-Oauth-Scopes":          []string{"repo"},
			},
		}
		assert.NotPanics(t, func() {
			assert.Equal(t,
				`This API operation needs the "read:org" scope. To request it, run:  gh auth refresh -h  -s read:org`,
				err.ScopesSuggestion())
		})
	})
}

func TestScopesSuggestion(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		acceptedScopes string
		tokenScopes    string
		requestURL     string
		wantSuggestion string
	}{
		{
			name:           "no suggestion on 2xx",
			statusCode:     200,
			acceptedScopes: "repo, read:org",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "no suggestion on 5xx",
			statusCode:     500,
			acceptedScopes: "repo, read:org",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "no suggestion on 422",
			statusCode:     422,
			acceptedScopes: "repo, read:org",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
		},
		{
			// The caller that renders this suggestion has a bug that hides it
			// on a 401. That is recorded in the design spec and lives outside
			// this package, so the behaviour here is unchanged.
			name:           "suggestion on 401",
			statusCode:     401,
			acceptedScopes: "repo, read:org",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
			wantSuggestion: `This API operation needs the "read:org" scope. To request it, run:  gh auth refresh -h github.com -s read:org`,
		},
		{
			name:           "suggestion on 403",
			statusCode:     403,
			acceptedScopes: "repo, read:org",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
			wantSuggestion: `This API operation needs the "read:org" scope. To request it, run:  gh auth refresh -h github.com -s read:org`,
		},
		{
			name:           "suggestion on 404",
			statusCode:     404,
			acceptedScopes: "repo, read:org",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
			wantSuggestion: `This API operation needs the "read:org" scope. To request it, run:  gh auth refresh -h github.com -s read:org`,
		},
		{
			name:           "no suggestion when the token has no scopes",
			statusCode:     404,
			acceptedScopes: "repo, read:org",
			tokenScopes:    "",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "no suggestion when the endpoint needs no scopes",
			statusCode:     404,
			acceptedScopes: "",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "no suggestion when the token already has the scope",
			statusCode:     404,
			acceptedScopes: "repo",
			tokenScopes:    "repo, read:org",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "repo implies its grouped scopes",
			statusCode:     404,
			acceptedScopes: "public_repo",
			tokenScopes:    "repo",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "user implies its grouped scopes",
			statusCode:     404,
			acceptedScopes: "read:user",
			tokenScopes:    "user",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "codespace implies codespace:secrets",
			statusCode:     404,
			acceptedScopes: "codespace:secrets",
			tokenScopes:    "codespace",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "admin implies read and write",
			statusCode:     404,
			acceptedScopes: "read:org, write:org",
			tokenScopes:    "admin:org",
			requestURL:     "https://api.github.com/gists",
		},
		{
			name:           "write implies read",
			statusCode:     404,
			acceptedScopes: "read:org",
			tokenScopes:    "write:org",
			requestURL:     "https://api.github.com/gists",
		},
		{
			// The other cases already show api.github.com collapsing to
			// github.com. An enterprise host has no such suffix and is used
			// as it stands.
			name:           "an enterprise hostname is used as it stands",
			statusCode:     404,
			acceptedScopes: "read:org",
			tokenScopes:    "repo",
			requestURL:     "https://github.example.com/api/v3/gists",
			wantSuggestion: `This API operation needs the "read:org" scope. To request it, run:  gh auth refresh -h github.example.com -s read:org`,
		},
		{
			name:           "a tenancy hostname is normalized to the tenant",
			statusCode:     404,
			acceptedScopes: "read:org",
			tokenScopes:    "repo",
			requestURL:     "https://api.tenant.ghe.com/gists",
			wantSuggestion: `This API operation needs the "read:org" scope. To request it, run:  gh auth refresh -h tenant.ghe.com -s read:org`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestURL, err := url.Parse(tt.requestURL)
			require.NoError(t, err)

			errResp := &ErrorResponse{
				Headers: http.Header{
					"X-Accepted-Oauth-Scopes": []string{tt.acceptedScopes},
					"X-Oauth-Scopes":          []string{tt.tokenScopes},
				},
				RequestURL: requestURL,
				StatusCode: tt.statusCode,
			}

			assert.Equal(t, tt.wantSuggestion, errResp.ScopesSuggestion())
		})
	}
}

// TestNewErrorResponse covers the exported parser, which exists for callers
// that already hold a response, such as api.HandleHTTPError.
func TestNewErrorResponse(t *testing.T) {
	jsonHeader := func() http.Header {
		return http.Header{"Content-Type": []string{"application/json"}}
	}

	newResponse := func(status int, header http.Header, body string, req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}
	}

	t.Run("parses the payload from a response the caller holds", func(t *testing.T) {
		requestURL, err := url.Parse("https://api.github.com/repos/OWNER/REPO/issues")
		require.NoError(t, err)
		req := &http.Request{URL: requestURL}

		body := `{"message":"Validation Failed","errors":[{"resource":"Issue","field":"title","code":"missing_field"}]}`
		resp := newResponse(http.StatusUnprocessableEntity, jsonHeader(), body, req)

		errResp := NewErrorResponse(resp)

		assert.Equal(t, http.StatusUnprocessableEntity, errResp.StatusCode)
		assert.Equal(t, "Validation Failed\nIssue.title is missing", errResp.Message)
		assert.Equal(t, []ErrorItem{{Resource: "Issue", Field: "title", Code: "missing_field"}}, errResp.Errors)
		assert.Equal(t, requestURL, errResp.RequestURL)
		assert.Equal(t, "application/json", errResp.Headers.Get("Content-Type"))
	})

	t.Run("falls back to the status for a non-JSON body", func(t *testing.T) {
		requestURL, err := url.Parse("https://api.github.com/user")
		require.NoError(t, err)
		header := http.Header{"Content-Type": []string{"text/plain"}}

		resp := newResponse(http.StatusBadGateway, header, "oh no", &http.Request{URL: requestURL})

		errResp := NewErrorResponse(resp)

		assert.Equal(t, http.StatusBadGateway, errResp.StatusCode)
		assert.Equal(t, http.StatusText(http.StatusBadGateway), errResp.Message)
	})

	t.Run("does not close the body", func(t *testing.T) {
		requestURL, err := url.Parse("https://api.github.com/user")
		require.NoError(t, err)

		body := newTrackedBody(`{"message":"Not Found"}`)
		resp := newResponse(http.StatusNotFound, jsonHeader(), "", &http.Request{URL: requestURL})
		resp.Body = body

		errResp := NewErrorResponse(resp)

		assert.Equal(t, "Not Found", errResp.Message)
		assert.False(t, body.isClosed(), "closing the body is the caller's decision")
	})

	// go-gh's HandleHTTPError dereferences resp.Request.URL unguarded. This
	// path is unreachable through httpmock, whose responders all set Request,
	// so the response is built by hand rather than through a test double.
	t.Run("tolerates a nil Request without mutating the caller's response", func(t *testing.T) {
		resp := newResponse(http.StatusNotFound, jsonHeader(), `{"message":"Not Found"}`, nil)

		var errResp *ErrorResponse
		require.NotPanics(t, func() {
			errResp = NewErrorResponse(resp)
		})

		assert.Equal(t, http.StatusNotFound, errResp.StatusCode)
		assert.Equal(t, "Not Found", errResp.Message)
		assert.Nil(t, resp.Request, "the caller's response must be left as it was")
	})

	// An empty *url.URL would satisfy every dereference and then print as an
	// empty string, so an error would read "HTTP 404: Not Found ()" as though a
	// real URL were blank. A nil one prints as <nil>, which is visibly absent.
	t.Run("leaves RequestURL nil rather than empty when Request is nil", func(t *testing.T) {
		resp := newResponse(http.StatusNotFound, jsonHeader(), `{"message":"Not Found"}`, nil)

		errResp := NewErrorResponse(resp)

		assert.Nil(t, errResp.RequestURL)
		assert.NotPanics(t, func() {
			assert.Equal(t, "HTTP 404: Not Found (<nil>)", errResp.Error())
		})
		assert.Equal(t, "", errResp.ScopesSuggestion())

		empty := &ErrorResponse{Message: "Not Found", RequestURL: &url.URL{}, StatusCode: 404}
		assert.Equal(t, "HTTP 404: Not Found ()", empty.Error(),
			"an empty URL would read as a real but blank URL, which is why nil is kept")
	})
}
