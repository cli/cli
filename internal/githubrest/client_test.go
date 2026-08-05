package githubrest

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
func stubClient(t *testing.T, baseURL string, auth AuthStrategy, respFn func(*http.Request) *http.Response) (*Client, **http.Request) {
	t.Helper()

	var sent *http.Request
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sent = req
		return respFn(req), nil
	})}

	client, err := NewClient(baseURL, httpClient, auth)
	require.NoError(t, err)

	return client, &sent
}

// newTestClient mirrors NewClient, taking the auth strategy explicitly so that
// tests state it as production code has to.
func newTestClient(t *testing.T, baseURL string, httpClient *http.Client, auth AuthStrategy, opts ...ClientOption) *Client {
	t.Helper()

	client, err := NewClient(baseURL, httpClient, auth, opts...)
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
			name:    "absolute URL on the same host is passed through, as Link headers supply",
			baseURL: "https://api.github.com/",
			path:    "https://api.github.com/repositories/1/issues?page=2",
			wantURL: "https://api.github.com/repositories/1/issues?page=2",
		},
		{
			name:    "absolute URL on the same host is compared case insensitively",
			baseURL: "https://api.github.com/",
			path:    "https://API.GitHub.com/repos/OWNER/REPO",
			wantURL: "https://API.GitHub.com/repos/OWNER/REPO",
		},
		{
			name:    "absolute URL on the enterprise upload path, which shares the host",
			baseURL: "https://github.example.com/api/v3/",
			path:    "https://github.example.com/api/uploads/repos/OWNER/REPO/releases/1/assets",
			wantURL: "https://github.example.com/api/uploads/repos/OWNER/REPO/releases/1/assets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.baseURL, nil, WithoutToken())

			req, err := client.NewRequest(context.Background(), http.MethodGet, tt.path, nil)
			require.NoError(t, err)

			assert.Equal(t, tt.wantURL, req.URL.String())
		})
	}
}

// A cross-host absolute URL would send the client's token to a host the caller
// never configured, and such URLs typically come from a response rather than
// from the caller, so they are rejected rather than followed.
func TestNewRequestRejectsCrossHostAbsoluteURLs(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		path    string
		wantErr string
	}{
		{
			name:    "a different host",
			baseURL: "https://api.github.com/",
			path:    "https://uploads.github.com/repos/OWNER/REPO/releases/1/assets?name=x",
			wantErr: "declare it with WithCredentialedHost",
		},
		{
			name:    "an unrelated host",
			baseURL: "https://api.github.com/",
			path:    "https://evil.example.com/repos/OWNER/REPO",
			wantErr: "declare it with WithCredentialedHost",
		},
		{
			name:    "a credentialed host over plaintext, which would expose the token",
			baseURL: "https://api.github.com/",
			path:    "http://api.github.com/repos/OWNER/REPO",
			wantErr: "over http when the client uses https",
		},
		{
			name:    "the same hostname on another port",
			baseURL: "https://github.example.com/api/v3/",
			path:    "https://github.example.com:8443/api/v3/repos/OWNER/REPO",
			wantErr: "declare it with WithCredentialedHost",
		},
		{
			name:    "an enterprise host when the client is configured for github.com",
			baseURL: "https://api.github.com/",
			path:    "https://github.example.com/api/v3/repos/OWNER/REPO",
			wantErr: "declare it with WithCredentialedHost",
		},
		{
			name:    "an absolute URL that cannot be parsed, so its host is unknowable",
			baseURL: "https://api.github.com/",
			path:    "https://api.github.com:notaport/repos/OWNER/REPO",
			wantErr: "parsing path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.baseURL, nil, WithToken("SECRET"))

			req, err := client.NewRequest(context.Background(), http.MethodGet, tt.path, nil)

			require.Error(t, err)
			assert.Nil(t, req, "no request should be built, so the token cannot be sent")
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// The REST upload endpoint is a separate host on github.com and a path on the
// same host on Enterprise Server. WithCredentialedHost exists so that one call
// site works on both, with the difference confined to construction.
func TestWithCredentialedHost(t *testing.T) {
	const uploadPath = "releases/1/assets?name=example.zip"

	t.Run("github.com reaches the upload host once it is declared", func(t *testing.T) {
		client := newTestClient(t, "https://api.github.com/", nil,
			WithToken("SECRET"),
			WithCredentialedHost("uploads.github.com"),
		)

		req, err := client.NewRequest(context.Background(), http.MethodPost, "https://uploads.github.com/repos/OWNER/REPO/"+uploadPath, nil)
		require.NoError(t, err)

		assert.Equal(t, "https://uploads.github.com/repos/OWNER/REPO/"+uploadPath, req.URL.String())
		assert.Equal(t, "token SECRET", req.Header.Get("Authorization"))
	})

	t.Run("enterprise reaches the upload path with no declaration needed", func(t *testing.T) {
		client := newTestClient(t, "https://github.example.com/api/v3/", nil, WithToken("SECRET"))

		req, err := client.NewRequest(context.Background(), http.MethodPost, "https://github.example.com/api/uploads/repos/OWNER/REPO/"+uploadPath, nil)
		require.NoError(t, err)

		assert.Equal(t, "https://github.example.com/api/uploads/repos/OWNER/REPO/"+uploadPath, req.URL.String())
		assert.Equal(t, "token SECRET", req.Header.Get("Authorization"))
	})

	t.Run("a declared host is matched case insensitively", func(t *testing.T) {
		client := newTestClient(t, "https://api.github.com/", nil, WithoutToken(), WithCredentialedHost("Uploads.GitHub.com"))

		req, err := client.NewRequest(context.Background(), http.MethodPost, "https://uploads.github.com/repos/OWNER/REPO/"+uploadPath, nil)
		require.NoError(t, err)

		assert.Equal(t, "https://uploads.github.com/repos/OWNER/REPO/"+uploadPath, req.URL.String())
	})

	t.Run("declaring one host does not credential another", func(t *testing.T) {
		client := newTestClient(t, "https://api.github.com/", nil, WithoutToken(), WithCredentialedHost("uploads.github.com"))

		_, err := client.NewRequest(context.Background(), http.MethodGet, "https://evil.example.com/repos/OWNER/REPO", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing to send credentials")
	})

	t.Run("a declared host cannot be reached over plaintext", func(t *testing.T) {
		client := newTestClient(t, "https://api.github.com/", nil, WithoutToken(), WithCredentialedHost("uploads.github.com"))

		_, err := client.NewRequest(context.Background(), http.MethodPost, "http://uploads.github.com/repos/OWNER/REPO/"+uploadPath, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "over http when the client uses https")
	})
}

func TestNewRequestAuthorization(t *testing.T) {
	tests := []struct {
		name        string
		auth        AuthStrategy
		opts        []RequestOption
		wantAuth    string
		wantAuthSet bool
	}{
		{
			name:        "WithoutToken leaves Authorization unset",
			auth:        WithoutToken(),
			wantAuthSet: false,
		},
		{
			name:        "WithToken sets Authorization",
			auth:        WithToken("CLIENT-TOKEN"),
			wantAuth:    "token CLIENT-TOKEN",
			wantAuthSet: true,
		},
		{
			name:        "WithToken with an empty token still sets Authorization",
			auth:        WithToken(""),
			wantAuth:    "token ",
			wantAuthSet: true,
		},
		{
			name:        "WithSingleRequestToken overrides the client token",
			auth:        WithToken("CLIENT-TOKEN"),
			opts:        []RequestOption{WithSingleRequestToken("OTHER-TOKEN")},
			wantAuth:    "token OTHER-TOKEN",
			wantAuthSet: true,
		},
		{
			name:        "WithSingleRequestToken supplies a token the client does not set itself",
			auth:        WithoutToken(),
			opts:        []RequestOption{WithSingleRequestToken("OTHER-TOKEN")},
			wantAuth:    "token OTHER-TOKEN",
			wantAuthSet: true,
		},
		{
			name:        "WithSingleRequestToken with an empty token still sets Authorization",
			auth:        WithToken("CLIENT-TOKEN"),
			opts:        []RequestOption{WithSingleRequestToken("")},
			wantAuth:    "token ",
			wantAuthSet: true,
		},
		{
			name:        "WithHeader can override Authorization",
			auth:        WithToken("CLIENT-TOKEN"),
			opts:        []RequestOption{WithHeader("Authorization", "SET-BY-HEADER")},
			wantAuth:    "SET-BY-HEADER",
			wantAuthSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, "https://api.github.com/", nil, tt.auth)

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
		name string
		auth AuthStrategy
		req  []RequestOption
	}{
		{
			name: "client token",
			auth: WithToken(""),
		},
		{
			name: "request token",
			auth: WithoutToken(),
			req:  []RequestOption{WithSingleRequestToken("")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, "https://api.github.com/", nil, tt.auth)

			req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil, tt.req...)
			require.NoError(t, err)

			assert.NotEmpty(t, req.Header.Get("Authorization"),
				"an empty Authorization value would let the transport inject the active user's token")
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Run("rejects an empty base URL", func(t *testing.T) {
		_, err := NewClient("", nil, WithoutToken())
		require.Error(t, err)
	})

	t.Run("rejects a relative base URL", func(t *testing.T) {
		_, err := NewClient("api.github.com", nil, WithoutToken())
		require.Error(t, err)
	})

	t.Run("rejects an unparseable base URL", func(t *testing.T) {
		_, err := NewClient("https://api.github.com/\x7f", nil, WithoutToken())
		require.Error(t, err)
	})

	t.Run("accepts an absolute base URL", func(t *testing.T) {
		client, err := NewClient("https://api.github.com/", nil, WithoutToken())
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	// Omitting auth entirely is a compile error, so the only way to reach this
	// is to pass an explicit nil, which would otherwise panic in NewClient.
	t.Run("rejects a nil auth strategy", func(t *testing.T) {
		_, err := NewClient("https://api.github.com/", nil, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "WithToken")
		assert.Contains(t, err.Error(), "WithoutToken")
	})

	t.Run("accepts a client that declares it sets no token", func(t *testing.T) {
		client, err := NewClient("https://api.github.com/", nil, WithoutToken())
		require.NoError(t, err)

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		_, ok := req.Header["Authorization"]
		assert.False(t, ok, "no Authorization header should be set")
	})
}

func TestNewRequestOptions(t *testing.T) {
	client := newTestClient(t, "https://api.github.com/", nil, WithoutToken())

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

	client := newTestClient(t, "https://api.github.com/", httpClient, WithToken("CLIENT-TOKEN"))

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
		client, sent := stubClient(t, "https://api.github.com/", WithToken("TOKEN"), func(req *http.Request) *http.Response {
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
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusCreated, io.NopCloser(strings.NewReader("{}")), nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodPost, "repos/OWNER/REPO/issues", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("returns the response alongside the error, with the body readable", func(t *testing.T) {
		body := newTrackedBody(`{"message":"Not Found"}`)
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
			header := http.Header{"Content-Type": []string{"application/json"}}
			return stubResponse(req, http.StatusNotFound, body, header)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		resp, sendErr := client.Send(req)
		require.Error(t, sendErr)
		require.NotNil(t, resp, "callers that read a 404 as a false, or print the raw body, need the response")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		// gh api prints exactly what the server sent, so parsing the error must
		// not have consumed the body.
		read, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, `{"message":"Not Found"}`, string(read))

		assert.True(t, body.isClosed(), "the original body is closed once buffered")

		var errResp *ErrorResponse
		require.ErrorAs(t, sendErr, &errResp)
		assert.Equal(t, "Not Found", errResp.Message)
	})

	t.Run("returns transport errors unchanged", func(t *testing.T) {
		wantErr := assert.AnError
		httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, wantErr
		})}
		client := newTestClient(t, "https://api.github.com/", httpClient, WithoutToken())

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
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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
				client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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

	// JSON can already say "nothing" as null, so an empty body is a malformed
	// response rather than an empty answer, and a caller that asked for a value
	// is better served by an error than by a zero-valued struct.
	t.Run("an empty body with a non-nil v is an error naming the cause", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("")), nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		var target struct{}
		resp, err := client.Do(req, &target)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty body")
		assert.Contains(t, err.Error(), "200", "the status is worth knowing, since a 2xx with no body is the surprise")
		require.NotNil(t, resp)
	})

	// null is valid JSON and means the answer really is nothing, so it decodes
	// rather than erroring, which is what distinguishes it from an empty body.
	t.Run("a null body decodes to the zero value", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("null")), nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		target := struct {
			Name string `json:"name"`
		}{Name: "untouched"}
		resp, err := client.Do(req, &target)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "untouched", target.Name)
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
				client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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
		require.NotNil(t, resp, "the status and headers are known good even when the status is not")
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

		var errResp *ErrorResponse
		require.ErrorAs(t, err, &errResp)
		assert.Equal(t, http.StatusUnprocessableEntity, errResp.StatusCode)
		assert.Equal(t, "Validation Failed\nIssue.title is missing", errResp.Message)
		assert.Equal(t, []ErrorItem{{Resource: "Issue", Field: "title", Code: "missing_field"}}, errResp.Errors)
		assert.Equal(t, "https://api.github.com/repos/OWNER/REPO/issues", errResp.RequestURL.String())
		assert.Equal(t, "application/json", errResp.Headers.Get("Content-Type"))
	})

	t.Run("a non-JSON error body falls back to the status", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
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
		client := newTestClient(t, server.URL, server.Client(), WithoutToken(),
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

	// gh release download rewrites a redirect target this way. Halting them
	// outright has its own method, SendIgnoringRedirects.
	t.Run("the policy can halt redirects, and a halted 302 is an ErrorResponse", func(t *testing.T) {
		server := newRedirectServer(t)

		client := newTestClient(t, server.URL, server.Client(), WithoutToken(),
			WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}))

		req, err := client.NewRequest(context.Background(), http.MethodGet, "legacy/asset", nil)
		require.NoError(t, err)

		resp, err := client.Send(req)
		require.Error(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()

		var errResp *ErrorResponse
		require.ErrorAs(t, err, &errResp)
		assert.Equal(t, http.StatusFound, errResp.StatusCode)
		assert.Equal(t, "/final/asset", errResp.Headers.Get("Location"))
	})

	t.Run("without the option redirects are followed normally", func(t *testing.T) {
		server := newRedirectServer(t)

		client := newTestClient(t, server.URL, server.Client(), WithoutToken())

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

		client := newTestClient(t, "https://api.github.com/", shared, WithoutToken(),
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

		client := newTestClient(t, "https://api.github.com/", shared, WithoutToken(),
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
		client := newTestClient(t, "https://api.github.com/", nil, WithoutToken(),
			WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
				return nil
			}))

		require.NotNil(t, client.http)
		assert.NotNil(t, client.http.CheckRedirect)
	})
}

// openerFor returns an opener over content, counting how many times it is
// called, which is how a replayed body shows up.
func openerFor(content string, calls *int) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) {
		*calls++
		return io.NopCloser(strings.NewReader(content)), nil
	}
}

func TestNewUploadRequest(t *testing.T) {
	ctx := context.Background()
	const uploadURL = "https://uploads.github.com/repos/OWNER/REPO/releases/1/assets?name=example.zip"

	uploadClient := func(t *testing.T) *Client {
		t.Helper()
		return newTestClient(t, "https://api.github.com/", nil,
			WithToken("SECRET"),
			WithCredentialedHost("uploads.github.com"),
		)
	}

	t.Run("sets everything an upload needs", func(t *testing.T) {
		calls := 0
		client := uploadClient(t)

		req, err := client.NewUploadRequest(ctx, uploadURL, openerFor("asset contents", &calls), 14, "application/zip")
		require.NoError(t, err)

		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, uploadURL, req.URL.String())
		assert.Equal(t, int64(14), req.ContentLength, "a missing length would send chunked encoding")
		assert.Equal(t, "application/zip", req.Header.Get("Content-Type"))
		assert.Equal(t, "token SECRET", req.Header.Get("Authorization"))
		require.NotNil(t, req.GetBody, "a missing GetBody would fail to replay on a redirect")
	})

	t.Run("GetBody reopens the body rather than replaying a consumed reader", func(t *testing.T) {
		calls := 0
		client := uploadClient(t)

		req, err := client.NewUploadRequest(ctx, uploadURL, openerFor("asset contents", &calls), 14, "application/zip")
		require.NoError(t, err)

		first, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, "asset contents", string(first))

		replay, err := req.GetBody()
		require.NoError(t, err)
		second, err := io.ReadAll(replay)
		require.NoError(t, err)

		assert.Equal(t, "asset contents", string(second), "the replayed body should be whole, not empty")
		assert.Equal(t, 2, calls, "GetBody should open the file again")
	})

	t.Run("options are applied after the upload headers, so they can override them", func(t *testing.T) {
		calls := 0
		client := uploadClient(t)

		req, err := client.NewUploadRequest(ctx, uploadURL, openerFor("x", &calls), 1, "application/zip",
			WithHeader("Content-Type", "application/octet-stream"),
		)
		require.NoError(t, err)

		assert.Equal(t, "application/octet-stream", req.Header.Get("Content-Type"))
	})

	// Nothing takes ownership of the reader when the request is never built, so
	// the opened file has to be closed here or it leaks.
	t.Run("the upload host must be credentialed like any other, and the opened body is closed", func(t *testing.T) {
		client := newTestClient(t, "https://api.github.com/", nil, WithToken("SECRET"))
		body := &recordingCloser{Reader: strings.NewReader("x")}

		_, err := client.NewUploadRequest(ctx, uploadURL, func() (io.ReadCloser, error) {
			return body, nil
		}, 1, "application/zip")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "refusing to send credentials")
		assert.True(t, body.closed, "the body opened before the failure should be closed")
	})

	t.Run("a body that cannot be opened is an error", func(t *testing.T) {
		client := uploadClient(t)

		_, err := client.NewUploadRequest(ctx, uploadURL, func() (io.ReadCloser, error) {
			return nil, errors.New("permission denied")
		}, 1, "application/zip")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})
}

func TestNewUploadRequestRequiresEveryPart(t *testing.T) {
	ctx := context.Background()
	const uploadURL = "https://api.github.com/repos/OWNER/REPO/releases/1/assets?name=x"

	okOpen := func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("x")), nil }

	tests := []struct {
		name        string
		open        func() (io.ReadCloser, error)
		size        int64
		contentType string
		wantErr     string
	}{
		{
			name:        "no opener",
			size:        1,
			contentType: "application/zip",
			wantErr:     "open is required",
		},
		{
			name:        "negative size",
			open:        okOpen,
			size:        -1,
			contentType: "application/zip",
			wantErr:     "is negative",
		},
		{
			name:    "no content type",
			open:    okOpen,
			size:    1,
			wantErr: "contentType is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, "https://api.github.com/", nil, WithToken("SECRET"))

			_, err := client.NewUploadRequest(ctx, uploadURL, tt.open, tt.size, tt.contentType)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// An empty file is a legitimate asset, so zero length must not be confused with
// a missing length.
func TestNewUploadRequestAllowsAnEmptyFile(t *testing.T) {
	calls := 0
	client := newTestClient(t, "https://api.github.com/", nil, WithToken("SECRET"))

	req, err := client.NewUploadRequest(context.Background(),
		"https://api.github.com/repos/OWNER/REPO/releases/1/assets?name=empty",
		openerFor("", &calls), 0, "application/octet-stream")
	require.NoError(t, err)

	assert.Equal(t, int64(0), req.ContentLength)
}

type recordingCloser struct {
	io.Reader
	closed bool
}

func (r *recordingCloser) Close() error {
	r.closed = true
	return nil
}

func TestSendIgnoringRedirects(t *testing.T) {
	newRedirectServer := func(t *testing.T) *httptest.Server {
		t.Helper()

		mux := http.NewServeMux()
		mux.HandleFunc("/repos/OWNER/REPO", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/repos/OWNER/RENAMED", http.StatusMovedPermanently)
		})
		mux.HandleFunc("/repos/OWNER/RENAMED", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)
		return server
	}

	// gh repo delete must not follow a rename redirect, because deleting
	// whatever a redirect points at is not what the user asked for.
	t.Run("stops at the first redirect and reports it as an ErrorResponse", func(t *testing.T) {
		server := newRedirectServer(t)
		client := newTestClient(t, server.URL, server.Client(), WithoutToken())

		req, err := client.NewRequest(context.Background(), http.MethodDelete, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		resp, err := client.SendIgnoringRedirects(req)
		require.Error(t, err)
		require.NotNil(t, resp)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)

		var errResp *ErrorResponse
		require.ErrorAs(t, err, &errResp)
		assert.Equal(t, "/repos/OWNER/RENAMED", errResp.Headers.Get("Location"))
	})

	t.Run("leaves the client's own redirect behaviour alone", func(t *testing.T) {
		server := newRedirectServer(t)
		client := newTestClient(t, server.URL, server.Client(), WithoutToken())

		ignoring, err := client.NewRequest(context.Background(), http.MethodDelete, "repos/OWNER/REPO", nil)
		require.NoError(t, err)
		_, err = client.SendIgnoringRedirects(ignoring)
		require.Error(t, err)

		following, err := client.NewRequest(context.Background(), http.MethodDelete, "repos/OWNER/REPO", nil)
		require.NoError(t, err)

		resp, err := client.Send(following)
		require.NoError(t, err, "Send must still follow redirects after SendIgnoringRedirects was used")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// The CLI hands one *http.Client to every command, so a policy set in place
	// rather than on a copy would leak into unrelated commands.
	t.Run("does not mutate the caller's http.Client", func(t *testing.T) {
		server := newRedirectServer(t)
		httpClient := server.Client()
		client := newTestClient(t, server.URL, httpClient, WithoutToken())

		req, err := client.NewRequest(context.Background(), http.MethodDelete, "repos/OWNER/REPO", nil)
		require.NoError(t, err)
		_, err = client.SendIgnoringRedirects(req)
		require.Error(t, err)

		assert.Nil(t, httpClient.CheckRedirect)
	})
}

func TestDoIntoWriter(t *testing.T) {
	// gh pr diff, gh run view --log and artifact downloads all want the bytes
	// the server sent, not a JSON document.
	t.Run("copies the raw body into an io.Writer", func(t *testing.T) {
		body := newTrackedBody("diff --git a/f b/f\n")
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, body, nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO/pulls/1", nil)
		require.NoError(t, err)

		var out strings.Builder
		resp, err := client.Do(req, &out)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "diff --git a/f b/f\n", out.String())
		assert.True(t, body.isClosed())
	})

	// The empty-body error exists to catch a JSON endpoint returning nothing.
	// A writer has no such expectation, so an empty body is simply no bytes.
	t.Run("an empty body is not an error for a writer", func(t *testing.T) {
		client, _ := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
			return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("")), nil)
		})

		req, err := client.NewRequest(context.Background(), http.MethodGet, "repos/OWNER/REPO/logs", nil)
		require.NoError(t, err)

		var out strings.Builder
		_, err = client.Do(req, &out)
		require.NoError(t, err)
		assert.Empty(t, out.String())
	})
}

func TestWithCacheTTL(t *testing.T) {
	client, captured := stubClient(t, "https://api.github.com/", WithoutToken(), func(req *http.Request) *http.Response {
		return stubResponse(req, http.StatusOK, io.NopCloser(strings.NewReader("{}")), nil)
	})

	req, err := client.NewRequest(context.Background(), http.MethodGet, "user", nil, WithCacheTTL(time.Hour))
	require.NoError(t, err)

	_, err = client.Do(req, nil)
	require.NoError(t, err)

	assert.Equal(t, "1h0m0s", (*captured).Header.Get("X-GH-CACHE-TTL"))
}

func TestPathTraversal(t *testing.T) {
	client := newTestClient(t, "https://api.github.com/", &http.Client{}, WithoutToken())

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "a relative path with a traversal segment", path: "repos/OWNER/REPO/../../user", wantErr: true},
		{name: "a percent-encoded traversal segment", path: "repos/OWNER/%2e%2e/user", wantErr: true},
		{name: "a leading traversal segment", path: "../user", wantErr: true},
		{name: "an absolute URL with a traversal segment", path: "https://api.github.com/repos/../user", wantErr: true},
		{name: "dots inside a segment are a legitimate name", path: "repos/OWNER/REPO/contents/file..txt", wantErr: false},
		{name: "a query string is not a path", path: "search/issues?q=a..b", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.NewRequest(context.Background(), http.MethodGet, tt.path, nil)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrPathTraversal)
				return
			}
			require.NoError(t, err)
		})
	}
}
