package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cli/cli/v2/internal/githubrest"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
)

const (
	apiVersion      = "X-GitHub-Api-Version"
	apiVersionValue = "2022-11-28"
	authorization   = "Authorization"
	cacheTTL        = "X-GH-CACHE-TTL"
	graphqlFeatures = "GraphQL-Features"
	features        = "merge_queue"
	userAgent       = "User-Agent"
)

func NewClientFromHTTP(httpClient *http.Client) *Client {
	client := &Client{http: httpClient}
	return client
}

type Client struct {
	http *http.Client
}

func (c *Client) HTTP() *http.Client {
	return c.http
}

// restClient builds a githubrest.Client for one host over this client's
// transport.
//
// The auth strategy is WithoutToken because the token is applied by the
// AddAuthTokenHeader transport that api.NewHTTPClient installs, exactly as it
// was before this package's REST calls moved onto githubrest. That transport
// does not overwrite an Authorization header that is already present, so a
// caller can still override per request.
//
// A githubrest client built over githubrest.NewHTTPClient carries no such
// transport, and there WithoutToken means what it says.
func (c Client) restClient(hostname string, opts ...githubrest.ClientOption) (*githubrest.Client, error) {
	opts = append([]githubrest.ClientOption{
		githubrest.WithCredentialedHost(githubrest.UploadHost(hostname)),
	}, opts...)

	return githubrest.NewClient(githubrest.APIBaseURL(hostname), c.http, githubrest.WithoutToken(), opts...)
}

// NewRESTClient returns a githubrest.Client for one host, built from an
// *http.Client the same way api.Client builds its own.
//
// It exists for the call sites that used to hand-build an *http.Request and
// call httpClient.Do, which need the request and the response rather than the
// decode-into-a-value shape of REST. Those call sites hold an *http.Client and
// a hostname and nothing else, so deriving the base URL has to happen for them
// somewhere, and doing it here keeps one implementation.
func NewRESTClient(httpClient *http.Client, hostname string, opts ...githubrest.ClientOption) (*githubrest.Client, error) {
	return NewClientFromHTTP(httpClient).restClient(hostname, opts...)
}

type GraphQLError struct {
	*ghAPI.GraphQLError
}

// GraphQL performs a GraphQL request using the query string and parses the response into data receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) GraphQL(hostname string, query string, variables map[string]interface{}, data interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Do(query, variables, data))
}

// Mutate performs a GraphQL mutation based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) Mutate(hostname, name string, mutation interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Mutate(name, mutation, variables))
}

// Query performs a GraphQL query based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) Query(hostname, name string, query interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Query(name, query, variables))
}

// QueryWithContext performs a GraphQL query based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) QueryWithContext(ctx context.Context, hostname, name string, query interface{}, variables map[string]interface{}) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.QueryWithContext(ctx, name, query, variables))
}

// REST performs a REST request and parses the response.
func (c Client) REST(hostname string, method string, p string, body io.Reader, data interface{}) error {
	return c.RESTWithContext(context.Background(), hostname, method, p, body, data)
}

// RESTWithContext performs a REST request with a caller-supplied context and
// parses the response.
func (c Client) RESTWithContext(ctx context.Context, hostname string, method string, p string, body io.Reader, data interface{}) error {
	restClient, err := c.restClient(hostname)
	if err != nil {
		return err
	}

	req, err := restClient.NewRequest(ctx, method, p, body)
	if err != nil {
		return err
	}

	// data is passed through even when it is nil, so that a nil receiver still
	// reaches json.Unmarshal and fails on an empty body exactly as it does
	// today. githubrest.Do only discards the body for an untyped nil, and every
	// caller here passes at least a typed one.
	_, err = restClient.Do(req, &data)
	return err
}

// RESTWithNext performs a REST request, parses the response, and returns the
// rel="next" URL from the Link header when the response carries one.
func (c Client) RESTWithNext(hostname string, method string, p string, body io.Reader, data interface{}) (string, error) {
	restClient, err := c.restClient(hostname)
	if err != nil {
		return "", err
	}

	req, err := restClient.NewRequest(context.Background(), method, p, body)
	if err != nil {
		return "", err
	}

	// 205 is deliberately not special-cased here even though githubrest.Do
	// skips decoding for it, matching this method's current behaviour rather
	// than REST's. A 205 previously reached json.Unmarshal and failed on the
	// empty body; it now returns "" with no error. No endpoint the CLI
	// paginates returns 205, so this is a difference in an unreachable branch,
	// but it is a difference.
	resp, err := restClient.Do(req, &data)
	if err != nil {
		return "", err
	}

	return resp.NextPage(), nil
}

// HandleHTTPError parses a http.Response into a *githubrest.ErrorResponse.
//
// The caller is responsible to close the response body stream.
func HandleHTTPError(resp *http.Response) error {
	return githubrest.NewErrorResponse(resp)
}

// handleResponse converts a ghAPI.GraphQLError into a GraphQLError.
//
// It used to convert ghAPI.HTTPError too. REST errors are now built by
// githubrest, which parses the payload with the same go-gh helper, so only the
// GraphQL half remains.
func handleResponse(err error) error {
	if err == nil {
		return nil
	}

	var gqlErr *ghAPI.GraphQLError
	if errors.As(err, &gqlErr) {
		return GraphQLError{
			GraphQLError: gqlErr,
		}
	}

	return err
}

// ScopesSuggestion is an error messaging utility that prints the suggestion to
// request additional OAuth scopes in case a server response indicates that
// there are missing scopes.
//
// It exists for gh api, which holds a raw *http.Response rather than an error,
// and it borrows githubrest's implementation rather than keeping a second copy
// of the scope-grouping rules.
func ScopesSuggestion(resp *http.Response) string {
	errResp := &githubrest.ErrorResponse{
		Headers:    resp.Header,
		StatusCode: resp.StatusCode,
	}
	if resp.Request != nil {
		errResp.RequestURL = resp.Request.URL
	}
	return errResp.ScopesSuggestion()
}

// ErrorNeedsScopes annotates a REST error with an OAuth scope the endpoint
// requires but does not advertise in its response, so that the resulting
// scope suggestion names it.
//
// It is the Send/Do shaped counterpart to EndpointNeedsScopes, which needs the
// *http.Response that those two no longer hand back on a failure.
func ErrorNeedsScopes(err error, s string) error {
	var errResp *githubrest.ErrorResponse
	if errors.As(err, &errResp) && errResp.StatusCode >= 400 && errResp.StatusCode < 500 {
		oldScopes := errResp.Headers.Get("X-Accepted-Oauth-Scopes")
		errResp.Headers.Set("X-Accepted-Oauth-Scopes", fmt.Sprintf("%s, %s", oldScopes, s))
	}
	return err
}

// clientOptions builds go-gh client options for one host.
//
// Only GraphQL uses this now, since REST is built on githubrest. AuthToken and
// the Authorization header are still handled by the AddAuthTokenHeader
// transport, so go-gh is told not to resolve them.
func clientOptions(hostname string, transport http.RoundTripper) ghAPI.ClientOptions {
	opts := ghAPI.ClientOptions{
		AuthToken: "none",
		Headers: map[string]string{
			authorization: "",
			apiVersion:    apiVersionValue,
		},
		Host:               hostname,
		SkipDefaultHeaders: true,
		Transport:          transport,
		LogIgnoreEnv:       true,
	}
	return opts
}

// NewRESTClientForURL returns a githubrest.Client for the host of an already
// absolute API URL.
//
// Several helpers are handed a URL that came from an earlier API response, such
// as a release's own URL, and never see a hostname. Deriving the host from the
// URL keeps their signatures as they are. The host is normalized during token
// lookup and base URL derivation, so api.github.com resolves github.com's token
// and base URL, and an Enterprise host resolves its own.
func NewRESTClientForURL(httpClient *http.Client, apiURL string, opts ...githubrest.ClientOption) (*githubrest.Client, error) {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("api: %q has no host, so no client can be built for it", apiURL)
	}
	return NewRESTClient(httpClient, parsed.Host, opts...)
}
