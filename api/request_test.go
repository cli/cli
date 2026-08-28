package api

import (
	"bytes"
	"context"

	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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

func TestRequestWithHeader(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(200, "{}"))

	resp, err := client.Request("github.com", http.MethodGet, "repos/OWNER/REPO", nil,
		WithHeader("Accept", "application/vnd.github.raw"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "application/vnd.github.raw", reg.Requests[0].Header.Get("Accept"))
}

// The transport supplies default headers of its own, so a per-request header is only useful if it
// takes precedence over them.
func TestRequestHeaderOverridesTransportDefault(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(200, "{}"))

	resp, err := client.Request("github.com", http.MethodPost, "repos/OWNER/REPO/releases", nil,
		WithHeader("Content-Type", "application/zip"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "application/zip", reg.Requests[0].Header.Get("Content-Type"))
}

// Pre-auth call sites supply their own token because it is not yet in config, so an explicit
// Authorization header must survive rather than be replaced by the transport.
func TestRequestExplicitAuthorizationHeaderIsPreserved(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	httpClient.Transport = AddAuthTokenHeader(httpClient.Transport, tinyConfig{"github.com:oauth_token": "config-token"})
	client := NewClientFromHTTP(httpClient)

	reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(200, "{}"))

	resp, err := client.Request("github.com", http.MethodGet, "user", nil,
		WithHeader("Authorization", "token explicit-token"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "token explicit-token", reg.Requests[0].Header.Get("Authorization"))
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

	reg.Register(httpmock.MatchAny, httpmock.StatusJSONResponse(404, map[string]any{
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

	var gotValue any
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

func TestDoRequest(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(201, `{"id": 1}`))

	// Fields such as ContentLength are the reason DoRequest exists, so assert one survives.
	req, err := http.NewRequest(http.MethodPost, "https://uploads.github.com/assets", strings.NewReader("longer-than-assigned-content-length"))
	require.NoError(t, err)
	req.ContentLength = 1

	resp, err := client.DoRequest(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, "uploads.github.com", reg.Requests[0].URL.Hostname())
	assert.Equal(t, int64(1), reg.Requests[0].ContentLength)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"id": 1}`, string(body))
}

func TestDoRequestError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, httpmock.StatusJSONResponse(422, map[string]any{
		"message": "Validation Failed",
	}))

	req, err := http.NewRequest(http.MethodPost, "https://uploads.github.com/assets", nil)
	require.NoError(t, err)

	resp, err := client.DoRequest(req)
	assert.Nil(t, resp)

	var httpErr HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, 422, httpErr.StatusCode)
	assert.Equal(t, "Validation Failed", httpErr.Message)
}

// The transport built by NewHTTPClient supplies default Accept, Content-Type and User-Agent
// headers. Per-request headers are only useful if they win against that real transport, so this
// exercises it rather than a bare test tripper.
func TestRequestHeadersAgainstRealTransport(t *testing.T) {
	var gotReq *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ios, _, _, _ := iostreams.Test()
	httpClient, err := NewHTTPClient(HTTPClientOptions{
		AppVersion: "v1.2.3",
		Config:     tinyConfig{serverHostname(t, ts.URL) + ":oauth_token": "MYTOKEN"},
		Log:        ios.ErrOut,
	})
	require.NoError(t, err)
	client := NewClientFromHTTP(httpClient)

	resp, err := client.Request(ts.URL, http.MethodGet, ts.URL+"/user/repos", nil,
		WithHeader("Accept", "application/vnd.github.raw"),
		WithHeader("Content-Type", "application/zip"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "application/vnd.github.raw", gotReq.Header.Get("Accept"))
	assert.Equal(t, "application/zip", gotReq.Header.Get("Content-Type"))

	// Headers the caller did not override must still be supplied by the transport.
	assert.Equal(t, "token MYTOKEN", gotReq.Header.Get("Authorization"))
	assert.Equal(t, "GitHub CLI v1.2.3", gotReq.Header.Get("User-Agent"))
}

func TestGraphQLWithHeader(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, httpmock.StatusStringResponse(200, `{"data": {}}`))

	var response struct{}
	err := client.GraphQL("github.com", "query{}", nil, &response,
		WithHeader("Authorization", "token explicit-token"))
	require.NoError(t, err)

	assert.Equal(t, "token explicit-token", reg.Requests[0].Header.Get("Authorization"))
}

// TestDoRequestPreservesCheckRedirect pins the difference between Request and DoRequest that
// motivates DoRequest existing for redirect-sensitive call sites. Request delegates to go-gh,
// which builds an http.Client of its own from the transport alone, so a redirect policy set on
// the client cannot survive. DoRequest sends through that client and so keeps it.
func TestDoRequestPreservesCheckRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	newClient := func(called *bool) *http.Client {
		return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			*called = true
			return http.ErrUseLastResponse
		}}
	}

	t.Run("DoRequest honours it", func(t *testing.T) {
		var called bool
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/start", nil)
		require.NoError(t, err)

		// The redirect is not followed, so the 3xx is the final response and, being outside
		// the 2xx range, is reported as an error. Call sites such as gh repo delete rely on
		// seeing that status to explain that a repo was renamed or transferred.
		_, err = NewClientFromHTTP(newClient(&called)).DoRequest(req)
		require.Error(t, err)

		var httpErr HTTPError
		require.ErrorAs(t, err, &httpErr)
		assert.True(t, called, "CheckRedirect should have been consulted")
		assert.Equal(t, http.StatusFound, httpErr.StatusCode)
	})

	t.Run("Request cannot honour it", func(t *testing.T) {
		var called bool

		resp, err := NewClientFromHTTP(newClient(&called)).Request("github.com", http.MethodGet, ts.URL+"/start", nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.False(t, called, "go-gh builds its own client, so the policy cannot apply")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
