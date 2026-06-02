package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingTripper records whether it was invoked and echoes back a 200 response.
type recordingTripper struct {
	called  bool
	gotBody string
}

func (rt *recordingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.called = true
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		rt.gotBody = string(b)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

func newRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, r)
	require.NoError(t, err)
	return req
}

func TestReadOnlyMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		url         string
		body        string
		wantBlocked bool
	}{
		{
			name:   "REST GET is allowed",
			method: http.MethodGet,
			url:    "https://api.github.com/repos/cli/cli",
		},
		{
			name:   "REST HEAD is allowed",
			method: http.MethodHead,
			url:    "https://api.github.com/repos/cli/cli",
		},
		{
			name:   "REST lowercase get is allowed",
			method: "get",
			url:    "https://api.github.com/repos/cli/cli",
		},
		{
			name:        "REST POST is blocked",
			method:      http.MethodPost,
			url:         "https://api.github.com/repos/cli/cli/issues",
			body:        `{"title":"x"}`,
			wantBlocked: true,
		},
		{
			name:        "REST PATCH is blocked",
			method:      http.MethodPatch,
			url:         "https://api.github.com/repos/cli/cli/issues/1",
			wantBlocked: true,
		},
		{
			name:        "REST PUT is blocked",
			method:      http.MethodPut,
			url:         "https://api.github.com/repos/cli/cli/issues/1/lock",
			wantBlocked: true,
		},
		{
			name:        "REST DELETE is blocked",
			method:      http.MethodDelete,
			url:         "https://api.github.com/repos/cli/cli/issues/1/lock",
			wantBlocked: true,
		},
		{
			name:   "GraphQL query is allowed",
			method: http.MethodPost,
			url:    "https://api.github.com/graphql",
			body:   `{"query":"query { viewer { login } }"}`,
		},
		{
			name:   "GraphQL anonymous query is allowed",
			method: http.MethodPost,
			url:    "https://api.github.com/graphql",
			body:   `{"query":"{ viewer { login } }"}`,
		},
		{
			name:        "GraphQL mutation is blocked",
			method:      http.MethodPost,
			url:         "https://api.github.com/graphql",
			body:        `{"query":"mutation { addStar(input:{starrableId:\"x\"}) { clientMutationId } }"}`,
			wantBlocked: true,
		},
		{
			name:        "GraphQL named mutation is blocked",
			method:      http.MethodPost,
			url:         "https://api.github.com/graphql",
			body:        `{"query":"mutation AddStar($id: ID!) { addStar(input:{starrableId:$id}) { clientMutationId } }"}`,
			wantBlocked: true,
		},
		{
			name:        "GraphQL mutation after a fragment is blocked",
			method:      http.MethodPost,
			url:         "https://api.github.com/graphql",
			body:        `{"query":"fragment f on Repository { name } mutation { addStar(input:{starrableId:\"x\"}) { clientMutationId } }"}`,
			wantBlocked: true,
		},
		{
			name:   "GraphQL query named mutation is allowed",
			method: http.MethodPost,
			url:    "https://api.github.com/graphql",
			body:   `{"query":"query mutation { viewer { login } }"}`,
		},
		{
			name:   "GraphQL enterprise endpoint query is allowed",
			method: http.MethodPost,
			url:    "https://ghe.example.com/api/graphql",
			body:   `{"query":"query { viewer { login } }"}`,
		},
		{
			name:        "GraphQL enterprise endpoint mutation is blocked",
			method:      http.MethodPost,
			url:         "https://ghe.example.com/api/graphql",
			body:        `{"query":"mutation { addStar(input:{starrableId:\"x\"}) { clientMutationId } }"}`,
			wantBlocked: true,
		},
		{
			name:        "unparseable GraphQL body fails closed",
			method:      http.MethodPost,
			url:         "https://api.github.com/graphql",
			body:        `not json`,
			wantBlocked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := &recordingTripper{}
			rt := ReadOnlyMiddleware(inner)
			req := newRequest(t, tt.method, tt.url, tt.body)

			res, err := rt.RoundTrip(req)

			if tt.wantBlocked {
				assert.ErrorIs(t, err, ErrReadOnly)
				assert.False(t, inner.called, "blocked request must not reach the network")
				return
			}

			require.NoError(t, err)
			assert.True(t, inner.called, "allowed request must reach the network")
			assert.Equal(t, http.StatusOK, res.StatusCode)
			// The body must survive inspection so the real request can be sent.
			if tt.body != "" {
				assert.Equal(t, tt.body, inner.gotBody)
			}
		})
	}
}

func TestNewHTTPClientReadOnly(t *testing.T) {
	t.Setenv("GH_READ_ONLY", "1")
	client, err := NewHTTPClient(HTTPClientOptions{})
	require.NoError(t, err)

	req := newRequest(t, http.MethodPost, "https://api.github.com/repos/cli/cli/issues", `{"title":"x"}`)
	_, err = client.Transport.RoundTrip(req)
	assert.ErrorIs(t, err, ErrReadOnly)
}

func TestNewHTTPClientReadOnlyDisabledByDefault(t *testing.T) {
	t.Setenv("GH_READ_ONLY", "")
	client, err := NewHTTPClient(HTTPClientOptions{})
	require.NoError(t, err)

	// With read-only off, the request should not be rejected by our middleware.
	// It may still fail to reach the (nonexistent) server, but never with ErrReadOnly.
	req := newRequest(t, http.MethodPost, "https://api.github.com/repos/cli/cli/issues", `{"title":"x"}`)
	_, err = client.Transport.RoundTrip(req)
	assert.False(t, errors.Is(err, ErrReadOnly))
}

func TestContainsTopLevelMutation(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "anonymous query", query: `{ viewer { login } }`},
		{name: "named query", query: `query Q { viewer { login } }`},
		{name: "query keyword only", query: `query { viewer { login } }`},
		{name: "subscription", query: `subscription { x }`},
		{name: "field named mutation", query: `query { mutation { login } }`, want: false},
		{name: "operation named mutation", query: `query mutation { viewer { login } }`, want: false},
		{name: "mutation", query: `mutation { addStar }`, want: true},
		{name: "named mutation", query: `mutation M { addStar }`, want: true},
		{name: "fragment then mutation", query: `fragment f on R { name } mutation { addStar }`, want: true},
		{name: "mutation in comment only", query: "# mutation\nquery { viewer { login } }", want: false},
		{name: "mutation in string only", query: `query { search(query: "mutation") { x } }`, want: false},
		{name: "block string contains mutation", query: `query { f(q: """mutation""") }`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, containsTopLevelMutation(tt.query))
		})
	}
}
