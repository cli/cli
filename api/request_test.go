package api

import (
	"bytes"
	"context"

	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestResolvesPathAgainstHost(t *testing.T) {
	tests := []struct {
		name         string
		hostname     string
		path         string
		wantHostname string
		wantPath     string
	}{
		{
			name:         "relative path resolves against github.com",
			hostname:     "github.com",
			path:         "repos/OWNER/REPO",
			wantHostname: "api.github.com",
			wantPath:     "/repos/OWNER/REPO",
		},
		{
			name:         "relative path resolves against an enterprise host",
			hostname:     "example.com",
			path:         "repos/OWNER/REPO",
			wantHostname: "example.com",
			wantPath:     "/api/v3/repos/OWNER/REPO",
		},
		{
			// Absolute URLs are how API-supplied locations such as asset and upload URLs
			// reach this method, so they must be requested exactly as given.
			name:         "absolute URL is requested as given",
			hostname:     "github.com",
			path:         "https://uploads.github.com/assets/1234",
			wantHostname: "uploads.github.com",
			wantPath:     "/assets/1234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			client := newTestClient(reg)

			reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(200, "{}"))

			resp, err := client.Request(tt.hostname, http.MethodGet, tt.path, nil)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantHostname, reg.Requests[0].URL.Hostname())
			assert.Equal(t, tt.wantPath, reg.Requests[0].URL.Path)
		})
	}
}

func TestRequestReturnsBodyUnread(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(200, "the raw body"))

	resp, err := client.Request("github.com", http.MethodGet, "repos/OWNER/REPO/contents/f", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "the raw body", string(body))
}

// REST treats 204 and 205 specially and decodes every other 2xx as JSON. Request must do neither,
// so that bodiless successes and non-JSON successes both work.
func TestRequestDoesNotDecodeSuccessfulResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "bodiless 200, as returned by HEAD", statusCode: 200, body: ""},
		{name: "no content", statusCode: 204, body: ""},
		{name: "non-JSON body", statusCode: 200, body: "d3f9c4a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			client := newTestClient(reg)

			reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(tt.statusCode, tt.body))

			resp, err := client.Request("github.com", http.MethodGet, "repos/OWNER/REPO", nil)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.statusCode, resp.StatusCode)
		})
	}
}

func TestRequestError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, httpmock.StatusJSONResponse(404, map[string]interface{}{
		"message": "Not Found",
	}))

	resp, err := client.Request("github.com", http.MethodGet, "repos/OWNER/REPO", nil)
	assert.Nil(t, resp)

	var httpErr HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 404, httpErr.StatusCode)
}

func TestRequestErrorPreservesScopesSuggestion(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Request:    req,
			StatusCode: 404,
			Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Not Found"}`)),
			Header: map[string][]string{
				"Content-Type":            {"application/json; charset=utf-8"},
				"X-Accepted-Oauth-Scopes": {"delete_repo"},
				"X-Oauth-Scopes":          {"repo"},
			},
		}, nil
	})

	_, err := client.Request("github.com", http.MethodDelete, "repos/OWNER/REPO", nil)

	var httpErr HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Contains(t, httpErr.ScopesSuggestion(), "delete_repo")
}

// Call sites used to mutate the response with EndpointNeedsScopes before converting it to an error.
// Request converts internally, so WithEndpointScopes has to reproduce that suggestion.
func TestRequestWithEndpointScopes(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		acceptedScopes string
		wantSuggestion string
	}{
		{
			name:           "adds scopes the endpoint did not report",
			statusCode:     403,
			acceptedScopes: "",
			wantSuggestion: "delete_repo",
		},
		{
			name:           "appends to scopes the endpoint did report",
			statusCode:     403,
			acceptedScopes: "repo",
			wantSuggestion: "delete_repo",
		},
		{
			name:           "leaves 5xx errors alone",
			statusCode:     500,
			acceptedScopes: "",
			wantSuggestion: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			client := newTestClient(reg)

			reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
				header := map[string][]string{
					"Content-Type":   {"application/json; charset=utf-8"},
					"X-Oauth-Scopes": {"repo"},
				}
				if tt.acceptedScopes != "" {
					header["X-Accepted-Oauth-Scopes"] = []string{tt.acceptedScopes}
				}
				return &http.Response{
					Request:    req,
					StatusCode: tt.statusCode,
					Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Forbidden"}`)),
					Header:     header,
				}, nil
			})

			_, err := client.Request("github.com", http.MethodDelete, "repos/OWNER/REPO", nil,
				WithEndpointScopes("delete_repo"))

			var httpErr HTTPError
			require.ErrorAs(t, err, &httpErr)
			if tt.wantSuggestion == "" {
				assert.Empty(t, httpErr.ScopesSuggestion())
			} else {
				assert.Contains(t, httpErr.ScopesSuggestion(), tt.wantSuggestion)
			}
		})
	}
}

type ctxKey struct{}

// Call sites such as release asset upload run inside a retry loop that owns a context, so the
// context has to reach the outgoing request.
func TestRequestWithContextPropagatesContext(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	var gotValue interface{}
	reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
		gotValue = req.Context().Value(ctxKey{})
		return httpmock.StatusStringResponse(200, "{}")(req)
	})

	ctx := context.WithValue(context.Background(), ctxKey{}, "carried")

	resp, err := client.RequestWithContext(ctx, "github.com", http.MethodGet, "repos/OWNER/REPO", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "carried", gotValue)
}
