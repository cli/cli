package githubv3

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// trackedBody records whether it has been closed.
type trackedBody struct {
	io.Reader
	mu     sync.Mutex
	closed bool
}

func newTrackedBody(s string) *trackedBody {
	return &trackedBody{Reader: strings.NewReader(s)}
}

func (b *trackedBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}

func (b *trackedBody) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// failingBody fails on read, standing in for a connection that drops partway
// through a response body.
type failingBody struct{}

func newFailingBody() io.ReadCloser {
	return failingBody{}
}

func (failingBody) Read([]byte) (int, error) {
	return 0, errors.New("connection reset while reading body")
}

func (failingBody) Close() error {
	return nil
}

// stubResponse builds a response for the given request, as net/http's own
// transport does, including setting Request.
func stubResponse(req *http.Request, status int, body io.ReadCloser, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     header,
		Body:       body,
		Request:    req,
	}
}

// stubClient returns a client whose transport always replies with resp, along
// with a pointer that captures the request that was sent.
func stubClient(t *testing.T, baseURL, token string, respFn func(*http.Request) *http.Response) (*Client, **http.Request) {
	t.Helper()

	var sent *http.Request
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sent = req
		return respFn(req), nil
	})}

	client, err := NewClient(baseURL, httpClient, WithClientToken(token))
	require.NoError(t, err)

	return client, &sent
}

// newTestClient builds a client for the common case, failing the test if the
// base URL is not usable.
func newTestClient(t *testing.T, baseURL string, httpClient *http.Client, opts ...ClientOption) *Client {
	t.Helper()

	client, err := NewClient(baseURL, httpClient, opts...)
	require.NoError(t, err)
	return client
}

func TestNewRequestURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		path    string
		wantURL string
	}{
		{
			name:    "relative path",
			baseURL: "https://api.github.com/",
			path:    "repos/OWNER/REPO",
			wantURL: "https://api.github.com/repos/OWNER/REPO",
		},
		{
			name:    "relative path against a base URL without a trailing slash",
			baseURL: "https://api.github.com",
			path:    "repos/OWNER/REPO",
			wantURL: "https://api.github.com/repos/OWNER/REPO",
		},
		{
			name:    "relative path with a leading slash",
			baseURL: "https://api.github.com/",
			path:    "/repos/OWNER/REPO",
			wantURL: "https://api.github.com/repos/OWNER/REPO",
		},
		{
			name:    "relative path against an enterprise base URL",
			baseURL: "https://github.example.com/api/v3/",
			path:    "repos/OWNER/REPO",
			wantURL: "https://github.example.com/api/v3/repos/OWNER/REPO",
		},
		{
			name:    "relative path with a query string",
			baseURL: "https://api.github.com/",
			path:    "repos/OWNER/REPO/issues?state=open&per_page=100",
			wantURL: "https://api.github.com/repos/OWNER/REPO/issues?state=open&per_page=100",
		},
		{
			name:    "absolute https URL ignores the base URL",
			baseURL: "https://api.github.com/",
			path:    "https://uploads.github.com/repos/OWNER/REPO/releases/1/assets?name=x",
			wantURL: "https://uploads.github.com/repos/OWNER/REPO/releases/1/assets?name=x",
		},
		{
			name:    "absolute http URL ignores the base URL",
			baseURL: "https://api.github.com/",
			path:    "http://api.github.localhost/repos/OWNER/REPO",
			wantURL: "http://api.github.localhost/repos/OWNER/REPO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.baseURL, nil)

			req, err := client.NewRequest(context.Background(), http.MethodGet, tt.path, nil)
			require.NoError(t, err)

			assert.Equal(t, tt.wantURL, req.URL.String())
		})
	}
}

func TestNewRequestAuthorization(t *testing.T) {
	tests := []struct {
		name        string
		clientOpts  []ClientOption
		opts        []RequestOption
		wantAuth    string
		wantAuthSet bool
	}{
		{
			name:        "a client with no token leaves Authorization unset for the transport",
			wantAuthSet: false,
		},
		{
			name:        "WithClientToken sets Authorization",
			clientOpts:  []ClientOption{WithClientToken("CLIENT-TOKEN")},
			wantAuth:    "token CLIENT-TOKEN",
			wantAuthSet: true,
		},
		{
			name:        "WithClientToken with an empty token still sets Authorization",
			clientOpts:  []ClientOption{WithClientToken("")},
			wantAuth:    "token ",
			wantAuthSet: true,
		},
		{
			name:        "WithToken overrides the client token",
			clientOpts:  []ClientOption{WithClientToken("CLIENT-TOKEN")},
			opts:        []RequestOption{WithToken("OTHER-TOKEN")},
			wantAuth:    "token OTHER-TOKEN",
			wantAuthSet: true,
		},
		{
			name:        "WithToken sets Authorization on a client with no token",
			opts:        []RequestOption{WithToken("OTHER-TOKEN")},
			wantAuth:    "token OTHER-TOKEN",
			wantAuthSet: true,
		},
		{
			name:        "WithToken with an empty token still sets Authorization",
			clientOpts:  []ClientOption{WithClientToken("CLIENT-TOKEN")},
			opts:        []RequestOption{WithToken("")},
			wantAuth:    "token ",
			wantAuthSet: true,
		},
		{
			name:        "WithHeader can override Authorization",
			clientOpts:  []ClientOption{WithClientToken("CLIENT-TOKEN")},
			opts:        []RequestOption{WithHeader("Authorization", "SET-BY-HEADER")},
			wantAuth:    "SET-BY-HEADER",
			wantAuthSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, "https://api.github.com/", nil, tt.clientOpts...)

			req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil, tt.opts...)
			require.NoError(t, err)

			_, ok := req.Header["Authorization"]
			assert.Equal(t, tt.wantAuthSet, ok)
			assert.Equal(t, tt.wantAuth, req.Header.Get("Authorization"))
		})
	}
}

// TestEmptyTokenStandsDownTheTransport pins the mechanism that makes an empty
// token mean "authenticate as exactly this token" rather than "let the
// transport decide". api.AddAuthTokenHeader and go-gh's header round tripper
// both stand down only when Header.Get("Authorization") is non-empty, so a
// header set to "" would not stop either of them injecting the active user's
// token, and a command such as gh auth status would report on the wrong
// credentials.
func TestEmptyTokenStandsDownTheTransport(t *testing.T) {
	tests := []struct {
		name   string
		client []ClientOption
		req    []RequestOption
	}{
		{
			name:   "client token",
			client: []ClientOption{WithClientToken("")},
		},
		{
			name: "request token",
			req:  []RequestOption{WithToken("")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, "https://api.github.com/", nil, tt.client...)

			req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil, tt.req...)
			require.NoError(t, err)

			assert.NotEmpty(t, req.Header.Get("Authorization"),
				"an empty Authorization value would let the transport inject the active user's token")
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Run("rejects an empty base URL", func(t *testing.T) {
		_, err := NewClient("", nil)
		require.Error(t, err)
	})

	t.Run("rejects a relative base URL", func(t *testing.T) {
		_, err := NewClient("api.github.com", nil)
		require.Error(t, err)
	})

	t.Run("rejects an unparseable base URL", func(t *testing.T) {
		_, err := NewClient("https://api.github.com/\x7f", nil)
		require.Error(t, err)
	})

	t.Run("accepts an absolute base URL", func(t *testing.T) {
		client, err := NewClient("https://api.github.com/", nil)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})
}

func TestNewRequestOptions(t *testing.T) {
	client := newTestClient(t, "https://api.github.com/", nil)

	t.Run("WithHeader sets a header", func(t *testing.T) {
		req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil,
			WithHeader("Accept", "application/vnd.github.v3.diff"))
		require.NoError(t, err)

		assert.Equal(t, "application/vnd.github.v3.diff", req.Header.Get("Accept"))
	})

	t.Run("options are applied in order", func(t *testing.T) {
		req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil,
			WithHeader("Accept", "first"),
			WithHeader("Accept", "second"))
		require.NoError(t, err)

		assert.Equal(t, []string{"second"}, req.Header.Values("Accept"))
	})

	t.Run("a caller can add a second value for a header", func(t *testing.T) {
		req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil,
			WithHeader("Accept", "first"),
			func(req *http.Request) { req.Header.Add("Accept", "second") })
		require.NoError(t, err)

		assert.Equal(t, []string{"first", "second"}, req.Header.Values("Accept"))
	})

	t.Run("the request carries the context", func(t *testing.T) {
		type ctxKey struct{}
		ctx := context.WithValue(context.Background(), ctxKey{}, "value")

		req, err := client.NewRequest(ctx, http.MethodGet, "user", nil)
		require.NoError(t, err)

		assert.Equal(t, "value", req.Context().Value(ctxKey{}))
	})

	t.Run("an invalid method is an error", func(t *testing.T) {
		_, err := client.NewRequest(context.Background(), "bad method", "user", nil)
		require.Error(t, err)
	})
}

// TestRequestOptionSurvivesGoGHDefaultHeaders covers the mechanism that gives
// options precedence over defaults: go-gh's header round tripper skips any
// header already present on the request, so setting one in NewRequest wins even
// though the defaults are applied later, at RoundTrip time.
func TestRequestOptionSurvivesGoGHDefaultHeaders(t *testing.T) {
	var sent *http.Request
	recorder := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sent = req
		return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("{}")), nil), nil
	})

	httpClient, err := ghAPI.NewHTTPClient(ghAPI.ClientOptions{
		Host:         "github.com",
		AuthToken:    "GO-GH-TOKEN",
		Transport:    recorder,
		LogIgnoreEnv: true,
	})
	require.NoError(t, err)

	client := newTestClient(t, "https://api.github.com/", httpClient, WithClientToken("CLIENT-TOKEN"))

	req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil,
		WithHeader("Accept", "application/vnd.github.v3.diff"))
	require.NoError(t, err)

	_, err = client.Do(req, nil)
	require.NoError(t, err)

	require.NotNil(t, sent)
	assert.Equal(t, "application/vnd.github.v3.diff", sent.Header.Get("Accept"))
	assert.Equal(t, "token CLIENT-TOKEN", sent.Header.Get("Authorization"))
	assert.NotEmpty(t, sent.Header.Get("User-Agent"), "go-gh should still supply defaults we did not set")
}

func TestSend(t *testing.T) {
	t.Run("leaves the body open for the caller", func(t *testing.T) {
		body := newTrackedBody(`{"name":"REPO"}`)
		client, sent := stubClient(t, "https://api.github.com/", "TOKEN", func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, body, nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.NoError(t, err)
		require.NotNil(t, *sent)

		assert.False(t, body.isClosed())

		read, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"name":"REPO"}`, string(read))

		require.NoError(t, resp.Body.Close())
		assert.True(t, body.isClosed())
	})

	t.Run("exposes the success status code", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusCreated, io.NopCloser(strings.NewReader("{}")), nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodPost, "repos/OWNER/REPO/issues", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("closes the body on the error path", func(t *testing.T) {
		body := newTrackedBody(`{"message":"Not Found"}`)
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			header := http.Header{"Content-Type": []string{"application/json"}}
			return stubResponse(req, http.StatusNotFound, body, header)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.True(t, body.isClosed(), "Send must close the body when the caller gets no response to close")
	})

	t.Run("returns transport errors unchanged", func(t *testing.T) {
		wantErr := assert.AnError
		httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, wantErr
		})}
		client := newTestClient(t, "https://api.github.com/", httpClient)

		req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.ErrorIs(t, err, wantErr)
		assert.Nil(t, resp)

		var errResp *ErrorResponse
		assert.False(t, errors.As(err, &errResp), "a transport failure is not an API error response")
	})
}

func TestDo(t *testing.T) {
	t.Run("decodes the body and closes it", func(t *testing.T) {
		body := newTrackedBody(`{"name":"REPO"}`)
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, body, nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		var target struct {
			Name string `json:"name"`
		}
		resp, err := client.Do(req, &target)
		require.NoError(t, err)

		assert.Equal(t, "REPO", target.Name)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, body.isClosed())
	})

	t.Run("a nil v discards the body without decoding", func(t *testing.T) {
		body := newTrackedBody("this is not JSON")
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, body, nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		resp, err := client.Do(req, nil)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, body.isClosed())
	})

	t.Run("a malformed body is an error that still returns the response", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("not JSON")), nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		var target struct{}
		resp, err := client.Do(req, &target)
		require.Error(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	// The request succeeded and the status and headers are known good on each
	// of these paths, so only body handling failed and the caller keeps the
	// response.
	t.Run("returns the response alongside every post-success body error", func(t *testing.T) {
		tests := []struct {
			name string
			body func() io.ReadCloser
			v    any
		}{
			{
				name: "decode failure",
				body: func() io.ReadCloser { return io.NopCloser(strings.NewReader("not JSON")) },
				v:    &struct{}{},
			},
			{
				name: "read failure",
				body: func() io.ReadCloser { return newFailingBody() },
				v:    &struct{}{},
			},
			{
				name: "drain failure on the nil v path",
				body: func() io.ReadCloser { return newFailingBody() },
				v:    nil,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
					return stubResponse(req, http.StatusCreated, tt.body(), nil)
				})

				req, err := client.NewRequest(context.Background(), http.MethodPost, "repos/OWNER/REPO/issues", nil)
				require.NoError(t, err)

				resp, err := client.Do(req, tt.v)
				require.Error(t, err)
				require.NotNil(t, resp, "a response that was received successfully must be returned")
				assert.Equal(t, http.StatusCreated, resp.StatusCode)
			})
		}
	})

	// api.Client.REST and go-gh both surface this as an error today.
	// google/go-github swallows it instead, and matching that would silently
	// change behaviour at every existing call site.
	t.Run("an empty body with a non-nil v is an error", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("")), nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		var target struct{}
		resp, err := client.Do(req, &target)
		require.EqualError(t, err, "unexpected end of JSON input")
		require.NotNil(t, resp)
	})

	t.Run("skips decoding on statuses without content", func(t *testing.T) {
		tests := []struct {
			name   string
			status int
		}{
			{name: "204 No Content", status: http.StatusNoContent},
			{name: "205 Reset Content", status: http.StatusResetContent},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				body := newTrackedBody("this is not JSON")
				client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
					return stubResponse(req, tt.status, body, nil)
				})

				req, err := client.NewRequest(context.Background(), http.MethodDelete, "repos/OWNER/REPO", nil)
				require.NoError(t, err)

				var target struct{}
				resp, err := client.Do(req, &target)
				require.NoError(t, err)

				assert.Equal(t, tt.status, resp.StatusCode)
				assert.True(t, body.isClosed())
			})
		}
	})

	t.Run("a non-2xx status yields an ErrorResponse", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			header := http.Header{"Content-Type": []string{"application/json"}}
			body := io.NopCloser(strings.NewReader(`{
				"message": "Validation Failed",
				"errors": [{"resource":"Issue","field":"title","code":"missing_field"}]
			}`))
			return stubResponse(req, http.StatusUnprocessableEntity, body, header)
		})

		req, err := client.NewRequest(context.Background(), http.MethodPost, "repos/OWNER/REPO/issues", nil)
		require.NoError(t, err)

		var target struct{}
		resp, err := client.Do(req, &target)
		require.Error(t, err)
		assert.Nil(t, resp)

		var errResp *ErrorResponse
		require.ErrorAs(t, err, &errResp)
		assert.Equal(t, http.StatusUnprocessableEntity, errResp.StatusCode)
		assert.Equal(t, "Validation Failed\nIssue.title is missing", errResp.Message)
		assert.Equal(t, []ErrorItem{{Resource: "Issue", Field: "title", Code: "missing_field"}}, errResp.Errors)
		assert.Equal(t, "https://api.github.com/repos/OWNER/REPO/issues", errResp.RequestURL.String())
		assert.Equal(t, "application/json", errResp.Headers.Get("Content-Type"))
	})

	t.Run("a non-JSON error body falls back to the status", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			header := http.Header{"Content-Type": []string{"text/plain"}}
			return stubResponse(req, http.StatusBadGateway, io.NopCloser(strings.NewReader("oh no")), header)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil)
		require.NoError(t, err)

		_, err = client.Do(req, nil)

		var errResp *ErrorResponse
		require.ErrorAs(t, err, &errResp)
		assert.Equal(t, http.StatusBadGateway, errResp.StatusCode)
		assert.Equal(t, http.StatusText(http.StatusBadGateway), errResp.Message)
	})

	// net/http only populates Response.Request for its own transport, and
	// go-gh's HandleHTTPError dereferences it without checking.
	t.Run("does not panic when the transport leaves Response.Request nil", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", "", func(req *http.Request) *http.Response {
			header := http.Header{"Content-Type": []string{"application/json"}}
			resp := stubResponse(req, http.StatusNotFound, io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)), header)
			resp.Request = nil
			return resp
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		_, err = client.Do(req, nil)

		var errResp *ErrorResponse
		require.ErrorAs(t, err, &errResp)
		assert.Equal(t, "https://api.github.com/repos/OWNER/REPO", errResp.RequestURL.String())
	})
}

func TestWithCheckRedirect(t *testing.T) {
	// A real redirect chain, so the policy is exercised by net/http rather than
	// asserted against directly.
	newRedirectServer := func(t *testing.T) *httptest.Server {
		t.Helper()

		mux := http.NewServeMux()
		mux.HandleFunc("/legacy/asset", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/final/asset", http.StatusFound)
		})
		mux.HandleFunc("/final/asset", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"asset"}`))
		})
		mux.HandleFunc("/rewritten/asset", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"rewritten"}`))
		})

		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)
		return server
	}

	t.Run("the policy intercepts a redirect and can rewrite the target", func(t *testing.T) {
		server := newRedirectServer(t)

		var via int
		client := newTestClient(t, server.URL, server.Client(),
			WithCheckRedirect(func(req *http.Request, v []*http.Request) error {
				via = len(v)
				if len(v) == 1 {
					req.URL.Path = "/rewritten/asset"
				}
				return nil
			}))

		req, err := client.NewRequest(context.Background(), http.MethodGet, "legacy/asset", nil)
		require.NoError(t, err)

		var target struct {
			Name string `json:"name"`
		}
		resp, err := client.Do(req, &target)
		require.NoError(t, err)

		assert.Equal(t, 1, via, "the redirect policy must fire")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "rewritten", target.Name, "the policy's rewrite must take effect")
	})

	// gh repo delete halts redirects this way, and then treats anything over
	// 299 as an error. Send reports the halted 302 as an *ErrorResponse because
	// isSuccess is 200-299, which is the same outcome by a different route.
	t.Run("the policy can halt redirects, and a halted 302 is an ErrorResponse", func(t *testing.T) {
		server := newRedirectServer(t)

		client := newTestClient(t, server.URL, server.Client(),
			WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}))

		req, err := client.NewRequest(context.Background(), http.MethodGet, "legacy/asset", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.Error(t, err)
		assert.Nil(t, resp)

		var errResp *ErrorResponse
		require.ErrorAs(t, err, &errResp)
		assert.Equal(t, http.StatusFound, errResp.StatusCode)
		assert.Equal(t, "/final/asset", errResp.Headers.Get("Location"))
	})

	t.Run("without the option redirects are followed normally", func(t *testing.T) {
		server := newRedirectServer(t)

		client := newTestClient(t, server.URL, server.Client())

		req, err := client.NewRequest(context.Background(), http.MethodGet, "legacy/asset", nil)
		require.NoError(t, err)

		var target struct {
			Name string `json:"name"`
		}
		resp, err := client.Do(req, &target)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "asset", target.Name)
	})

	// NewClient stores the caller's *http.Client, and the CLI hands the same one
	// to every command, so setting CheckRedirect in place would install this
	// policy process-wide.
	t.Run("does not mutate the caller's http.Client", func(t *testing.T) {
		shared := &http.Client{}

		client := newTestClient(t, "https://api.github.com/", shared,
			WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}))

		assert.Nil(t, shared.CheckRedirect, "the caller's client must be left alone")
		assert.NotNil(t, client.http.CheckRedirect)
		assert.NotSame(t, shared, client.http)
	})

	t.Run("preserves the rest of the caller's http.Client", func(t *testing.T) {
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("{}")), nil), nil
		})
		shared := &http.Client{Transport: transport, Timeout: 42 * time.Second}

		client := newTestClient(t, "https://api.github.com/", shared,
			WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
				return nil
			}))

		assert.Equal(t, 42*time.Second, client.http.Timeout)

		req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil)
		require.NoError(t, err)

		resp, err := client.Do(req, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("tolerates a nil http.Client", func(t *testing.T) {
		client := newTestClient(t, "https://api.github.com/", nil,
			WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
				return nil
			}))

		require.NotNil(t, client.http)
		assert.NotNil(t, client.http.CheckRedirect)
	})
}
