package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/internal/githubrest"
	o "github.com/cli/cli/v2/pkg/option"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
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

// NewClientFromHTTP returns a Client that resolves each host's token from the
// token carrier on httpClient's transport chain, when it has one.
func NewClientFromHTTP(httpClient *http.Client) *Client {
	client := &Client{http: httpClient}
	return client
}

// NewClientWithToken returns a Client that authenticates every request as
// exactly the given token, rather than resolving one per host.
//
// It exists for callers checking a specific user's credentials rather than the
// host's active ones, which used to be expressed by wrapping the transport in
// AddAuthTokenHeader with a one-token config.
func NewClientWithToken(httpClient *http.Client, token string) *Client {
	return &Client{http: httpClient, token: o.Some(token)}
}

type Client struct {
	http *http.Client

	// token, when present, overrides per-host resolution for every request.
	token o.Option[string]
}

func (c *Client) HTTP() *http.Client {
	return c.http
}

// tokenFor returns the token this client authenticates as on hostname.
//
// The token used to be applied by the AddAuthTokenHeader transport, so
// api.Client never had to hold one. Now that requests carry it explicitly, the
// client has to resolve it, which it does from the transport chain unless it
// was constructed with a fixed token.
func (c Client) tokenFor(hostname string) string {
	if token, ok := c.token.Value(); ok {
		return token
	}
	if c.http == nil || c.http.Transport == nil {
		return ""
	}
	return tokenForHost(c.http.Transport, hostname)
}

// restClient builds a githubrest.Client for one host.
//
// It is constructed per call because githubrest.Client holds a host and
// api.Client.REST takes one per call. That was already true of the go-gh
// RESTClient this replaces, but the cost changes from rebuilding a layered
// transport chain to allocating a small struct.
func (c Client) restClient(hostname string) (*githubrest.Client, error) {
	auth := githubrest.WithoutToken()
	if token := c.tokenFor(hostname); token != "" {
		auth = githubrest.WithToken(token)
	}

	return githubrest.NewClient(
		githubrest.APIBaseURL(hostname),
		c.http,
		auth,
		githubrest.WithCredentialedHost(githubrest.UploadHost(hostname)),
	)
}

type GraphQLError struct {
	*ghAPI.GraphQLError
}

// GraphQL performs a GraphQL request using the query string and parses the response into data receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) GraphQL(hostname string, query string, variables map[string]interface{}, data interface{}) error {
	opts := clientOptions(hostname, c.tokenFor(hostname), c.http.Transport)
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
	opts := clientOptions(hostname, c.tokenFor(hostname), c.http.Transport)
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
	opts := clientOptions(hostname, c.tokenFor(hostname), c.http.Transport)
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
	opts := clientOptions(hostname, c.tokenFor(hostname), c.http.Transport)
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

// ScopesSuggestion is an error messaging utility that prints the suggestion to request additional OAuth
// scopes in case a server response indicates that there are missing scopes.
func ScopesSuggestion(resp *http.Response) string {
	return generateScopesSuggestion(resp.StatusCode,
		resp.Header.Get("X-Accepted-Oauth-Scopes"),
		resp.Header.Get("X-Oauth-Scopes"),
		resp.Request.URL.Hostname())
}

// EndpointNeedsScopes adds additional OAuth scopes to an HTTP response as if they were returned from the
// server endpoint. This improves HTTP 4xx error messaging for endpoints that don't explicitly list the
// OAuth scopes they need.
func EndpointNeedsScopes(resp *http.Response, s string) {
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		oldScopes := resp.Header.Get("X-Accepted-Oauth-Scopes")
		resp.Header.Set("X-Accepted-Oauth-Scopes", fmt.Sprintf("%s, %s", oldScopes, s))
	}
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

// clientOptions builds go-gh client options for one host.
//
// AuthToken used to be "none" with an empty Authorization header, because the
// AddAuthTokenHeader transport applied the token instead. That transport no
// longer sets headers, so the token is passed here and go-gh's header round
// tripper applies it, still only for requests to a host in the same domain as
// Host. An empty token leaves the header unset, which is the unauthenticated
// case.
//
// Only GraphQL uses this now. REST is built on githubrest.
func clientOptions(hostname, token string, transport http.RoundTripper) ghAPI.ClientOptions {
	headers := map[string]string{
		apiVersion: apiVersionValue,
	}

	// An empty AuthToken makes go-gh resolve one from the environment and fail
	// when there is none, so the placeholder it was always given is kept for
	// the unauthenticated case. The empty Authorization header then stops the
	// header round tripper from sending "token none", exactly as before.
	if token == "" {
		token = "none"
		headers[authorization] = ""
	}

	opts := ghAPI.ClientOptions{
		AuthToken:          token,
		Headers:            headers,
		Host:               hostname,
		SkipDefaultHeaders: true,
		Transport:          transport,
		LogIgnoreEnv:       true,
	}
	return opts
}
