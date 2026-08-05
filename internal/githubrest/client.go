// Package githubrest is a client for GitHub's REST API.
//
// The API is versioned "v3" in its URL paths, but the name is avoided here:
// GitHub now versions the REST API by date through the X-GitHub-Api-Version
// header, so "v3" describes the URL layout rather than the API a caller gets.
// The name also collides confusingly with github.com/shurcooL/githubv4, which
// is a GraphQL query DSL rather than a REST client.
package githubrest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	o "github.com/cli/cli/v2/pkg/option"
)

const (
	authorizationHeader = "Authorization"
	contentTypeHeader   = "Content-Type"

	// cacheTTLHeader is go-gh's cache control header, which its caching
	// transport reads and strips before the request leaves.
	cacheTTLHeader = "X-GH-CACHE-TTL"
)

// ErrPathTraversal is returned when a request path contains a ".." segment.
var ErrPathTraversal = errors.New("githubrest: path must not contain a .. segment")

// Client sends requests to a single GitHub REST API host.
//
// It holds an already-resolved API base URL: it reads no configuration and has
// no opinion about how a canonical host such as github.example.com becomes an
// API host. That resolution happens once, where the client is constructed.
type Client struct {
	apiBaseURL *url.URL
	token      o.Option[string]
	http       *http.Client

	// noRedirect is http with a policy that stops at the first redirect. It is
	// built at construction rather than on demand so that SendIgnoringRedirects
	// never mutates shared state, and so a caller cannot forget to copy.
	noRedirect *http.Client

	// credentialedHosts are the hosts this client will send an Authorization
	// header to, lowercased and including any port.
	credentialedHosts map[string]struct{}

	// defaultRequestOptions are applied to every request the client builds.
	defaultRequestOptions []RequestOption
}

// AuthStrategy states whether a client authenticates, and is a required
// argument to NewClient rather than an option.
//
// Authentication is the one decision that is wrong in both directions: a client
// that should authenticate but does not gets an unhelpful 404 for anything
// private, and a client that should not authenticate but does sends a user's
// token somewhere it was never meant to go. Neither failure looks like a
// missing option at the call site, so a caller is made to say which they want
// and the compiler asks the question.
type AuthStrategy func(*Client)

// WithToken authenticates the client as exactly the given token.
//
// An empty token still sets the Authorization header, so this means "the token
// I hold, even if it is empty" rather than "no token". Callers that check one
// specific user's credentials, rather than the host's active ones, depend on
// that: they must send exactly what they hold and get whatever answer it earns.
func WithToken(token string) AuthStrategy {
	return func(c *Client) {
		c.token = o.Some(token)
	}
}

// WithoutToken declares that the client sets no Authorization header of its own.
//
// This states the client's intent, which is as far as its reach goes. It is not
// a guarantee of anonymity: an http.Client can carry a transport that adds
// credentials to any request lacking them, and this package neither knows nor
// controls what transport it was given. A caller that needs certainty about
// what is sent has to be sure of the transport too.
func WithoutToken() AuthStrategy {
	return func(c *Client) {
		c.token = o.None[string]()
	}
}

// ClientOption configures a Client at construction.
type ClientOption func(*Client)

// WithCredentialedHost declares an additional host this client may send its
// token to.
//
// The API base URL's own host is always credentialed, so this is only needed
// for a second host in the same deployment. The REST upload endpoint is the
// case that exists: it lives at https://uploads.github.com/ on github.com but
// at https://HOST/api/uploads/ on Enterprise Server, so whether it is a
// different host is a deployment accident rather than anything a call site
// should have to reason about. Declaring it at construction, where the
// deployment is already known, keeps that difference out of call sites.
func WithCredentialedHost(host string) ClientOption {
	return func(c *Client) {
		c.credentialedHosts[strings.ToLower(host)] = struct{}{}
	}
}

// WithCheckRedirect sets the redirect policy for the client's requests.
//
// Stopping at the first redirect has a method of its own in
// SendIgnoringRedirects, so this is for the one caller that rewrites a redirect
// target rather than refusing it: gh release download, avoiding legacy Codeload
// paths. A redirect policy belongs to the http.Client rather than to a request,
// so it cannot be a RequestOption.
func WithCheckRedirect(fn func(*http.Request, []*http.Request) error) ClientOption {
	return func(c *Client) {
		c.http = withRedirectPolicy(c.http, fn)
	}
}

// WithDefaultRequestOptions applies opts to every request the client builds,
// before the request's own options, so a call site can still override them.
//
// This exists for options that describe the client's whole job rather than one
// request. Caching is the case that exists: gh extension browse wants every
// search response served from cache for a day, and the searcher it hands the
// client to builds the requests itself.
func WithDefaultRequestOptions(opts ...RequestOption) ClientOption {
	return func(c *Client) {
		c.defaultRequestOptions = append(c.defaultRequestOptions, opts...)
	}
}

// NewClient returns a Client that sends requests to apiBaseURL through
// httpClient, authenticating as auth describes.
//
// apiBaseURL is a resolved, absolute API base URL such as
// https://api.github.com/ or https://github.example.com/api/v3/, not a
// canonical host. It is validated here rather than per request, so a Client
// either resolves relative paths correctly or does not exist.
func NewClient(apiBaseURL string, httpClient *http.Client, auth AuthStrategy, opts ...ClientOption) (*Client, error) {
	if apiBaseURL == "" {
		return nil, errors.New("githubrest: apiBaseURL is required")
	}

	parsed, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("githubrest: parsing apiBaseURL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("githubrest: apiBaseURL %q is not absolute", apiBaseURL)
	}

	// Callers cannot omit auth, but they can pass a nil of the right type,
	// which would otherwise be a nil call below.
	if auth == nil {
		return nil, errors.New("githubrest: an auth strategy is required, so pass WithToken or WithoutToken")
	}

	client := &Client{
		apiBaseURL: parsed,
		http:       httpClient,
		credentialedHosts: map[string]struct{}{
			strings.ToLower(parsed.Host): {},
		},
	}

	auth(client)

	for _, opt := range opts {
		opt(client)
	}

	// Derived after opts, because WithCheckRedirect replaces client.http.
	client.noRedirect = withRedirectPolicy(client.http, func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	})

	return client, nil
}

// withRedirectPolicy returns a shallow copy of httpClient carrying fn as its
// redirect policy.
//
// The copy is not ceremony: the CLI hands one *http.Client to every command, so
// setting the field in place would install the policy process-wide for
// unrelated commands.
func withRedirectPolicy(httpClient *http.Client, fn func(*http.Request, []*http.Request) error) *http.Client {
	copied := &http.Client{}
	if httpClient != nil {
		c := *httpClient
		copied = &c
	}
	copied.CheckRedirect = fn
	return copied
}

// RequestOption modifies a request built by NewRequest.
//
// It receives the *http.Request itself, so it is not limited to single-valued
// headers, and a caller who needs something no option covers can mutate the
// request between NewRequest and Send.
type RequestOption func(*http.Request)

// WithHeader sets a request header, replacing any existing values for name.
func WithHeader(name, value string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set(name, value)
	}
}

// WithCacheTTL asks go-gh's caching transport to serve this request from cache
// for up to ttl.
//
// The CLI's existing mechanism, api.NewCachedHTTPClient, derives a whole
// *http.Client to set one header, which forces a caching decision to be made
// where the client is assembled rather than where the request is made. Caching
// is a property of the request, so it is expressed as one.
func WithCacheTTL(ttl time.Duration) RequestOption {
	return func(req *http.Request) {
		req.Header.Set(cacheTTLHeader, ttl.String())
	}
}

// Like WithToken, an empty token still sets the header. This exists for callers
// that hold one client but need a particular request to carry different
// credentials; a caller whose every request uses the same token should say so
// at construction instead.
func WithSingleRequestToken(token string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set(authorizationHeader, authorizationValue(token))
	}
}

// authorizationValue renders a token as an Authorization header value.
//
// An empty token deliberately yields a non-empty value, so that "authenticate
// as this empty token" is distinguishable from "no header was set".
func authorizationValue(token string) string {
	return fmt.Sprintf("token %s", token)
}

// NewRequest builds a request against the client's host.
//
// path may be a path relative to the client's API base URL, or an absolute URL
// on a credentialed host, which is what following a Link header pagination URL
// needs.
//
// The Authorization header is set from the client's token, when it has one,
// before opts are applied, so an option can override it.
//
// No other headers are set here. The CLI's transport applies User-Agent,
// X-GitHub-Api-Version and, unless they are skipped, Accept, Content-Type and
// Time-Zone at RoundTrip time, and skips any header already present on the
// request. An option therefore wins by being set earlier, not later, and
// setting defaults here would only risk double-setting them.
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader, opts ...RequestOption) (*http.Request, error) {
	resolved, err := c.resolveURL(path)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, method, resolved, body)
	if err != nil {
		return nil, err
	}

	if token, ok := c.token.Value(); ok {
		req.Header.Set(authorizationHeader, authorizationValue(token))
	}

	for _, opt := range c.defaultRequestOptions {
		opt(req)
	}

	for _, opt := range opts {
		opt(req)
	}

	return req, nil
}

// NewUploadRequest builds a request that uploads a file, such as a release
// asset.
//
// Every part of an upload is a required positional argument rather than an
// option, because an upload only works if all of it is applied, and each
// missing piece fails quietly and somewhere else: no length sends chunked
// encoding, which the upload endpoint rejects; no content type stores the asset
// as the wrong kind of file; and a body that cannot be replayed turns any
// redirect or retry into a failure after the request looked fine. Making these
// arguments means the compiler asks for them.
//
// open is called once for the body and kept as req.GetBody, so the request can
// be replayed. This is why it is a function rather than an io.Reader: a reader
// can be consumed only once. Callers that already model a file as something
// reopenable pass that directly.
//
// The returned request owns the reader that open produced, and sending the
// request closes it. A caller that builds a request and never sends it is
// responsible for closing the body itself.
//
// path is resolved and credentialed exactly as in NewRequest, so on github.com,
// where uploads live on their own host, the client needs WithCredentialedHost.
func (c *Client) NewUploadRequest(ctx context.Context, path string, open func() (io.ReadCloser, error), size int64, contentType string, opts ...RequestOption) (*http.Request, error) {
	if open == nil {
		return nil, errors.New("githubrest: open is required, because an upload body must be replayable")
	}
	if size < 0 {
		return nil, fmt.Errorf("githubrest: size %d is negative", size)
	}
	if contentType == "" {
		return nil, errors.New("githubrest: contentType is required, so pass application/octet-stream if nothing better is known")
	}

	body, err := open()
	if err != nil {
		return nil, fmt.Errorf("githubrest: opening upload body: %w", err)
	}

	req, err := c.NewRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		body.Close()
		return nil, err
	}

	req.ContentLength = size
	req.GetBody = open
	req.Header.Set(contentTypeHeader, contentType)

	// Applied last, as in NewRequest, so an option can override any of these.
	for _, opt := range opts {
		opt(req)
	}

	return req, nil
}

// Send issues req and returns the response with its body still open, so the
// caller must always close it.
//
// A non-2xx status yields both the *Response and an *ErrorResponse. Returning
// the response alongside the error is what google/go-github does, and it is
// what lets a caller that cares about a failure's details reach them without a
// second, parallel send method. The cases that need it are real: gh api streams
// an error body to stdout verbatim, gh repo delete and gh gist create attach
// accepted scopes to the error, and repoExists and publishedReleaseExists read
// a 404 as a false rather than a failure.
//
// Unlike go-github, the body is not closed on the error path and its content is
// not truncated. go-github caps an error body at 1 MiB because it has already
// consumed it into the error; here the body is restored so a caller can read it
// again, which gh api depends on to print exactly what the server sent. Leaving
// it open in every case also means one rule rather than two: if Send returned a
// response, close it.
func (c *Client) Send(req *http.Request) (*Response, error) {
	return c.send(c.http, req)
}

// SendIgnoringRedirects issues req without following redirects, returning the
// redirect response itself.
//
// A redirect policy belongs to an http.Client rather than to a request, so it
// cannot be a RequestOption, and mutating the shared client would install the
// policy process-wide. google/go-github resolves this the same way: it builds a
// second http.Client at construction and exposes a method that uses it. A named
// method also says what the caller wants, where a general policy setter would
// only say how they intend to get it.
func (c *Client) SendIgnoringRedirects(req *http.Request) (*Response, error) {
	return c.send(c.noRedirect, req)
}

func (c *Client) send(httpClient *http.Client, req *http.Request) (*Response, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// net/http only populates Response.Request for its own transport, so a
	// custom RoundTripper can leave it nil. Settling it here rather than in
	// NewErrorResponse means the parser has one guarantee to reason about
	// instead of two, and this response is ours to mutate.
	if resp.Request == nil {
		resp.Request = req
	}

	if !isSuccess(resp.StatusCode) {
		errResp, err := newErrorResponsePreservingBody(resp)
		if err != nil {
			return nil, err
		}
		return &Response{Response: resp}, errResp
	}

	return &Response{Response: resp}, nil
}

// newErrorResponsePreservingBody parses resp into an *ErrorResponse and leaves
// resp.Body readable from the start.
//
// Parsing consumes the body, so it is buffered and replaced. Buffering is
// bounded by what the server sent, and only on a failure, which is the price of
// letting a caller both read a structured error and print the raw one.
func newErrorResponsePreservingBody(resp *http.Response) (*ErrorResponse, error) {
	b, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}

	resp.Body = io.NopCloser(bytes.NewReader(b))
	errResp := NewErrorResponse(resp)
	resp.Body = io.NopCloser(bytes.NewReader(b))

	return errResp, nil
}

// Do issues req, reads the response body into v, and closes the body.
//
// v decides how the body is read. An io.Writer receives it verbatim, which is
// what diffs, patches, logs and downloaded archives need; anything else is a
// JSON decode target; and a nil v discards it. The io.Writer case follows
// google/go-github, and it is why callers that want raw bytes do not need a
// separate method.
//
// Statuses 204 and 205 skip reading, since they carry no content.
//
// Once a response has been received it is always returned, even alongside an
// error, including the *ErrorResponse for a non-2xx status. The request
// succeeded and the status and headers are known good. The body of such a
// response is already closed, since Do closes it unconditionally, so only its
// metadata is readable.
func (c *Client) Do(req *http.Request, v any) (*Response, error) {
	resp, err := c.Send(req)
	if resp == nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err != nil {
		return resp, err
	}

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusResetContent {
		return resp, nil
	}

	if w, ok := v.(io.Writer); ok {
		if _, err := io.Copy(w, resp.Body); err != nil {
			return resp, err
		}
		return resp, nil
	}

	if v == nil {
		// Drained rather than ignored: the transport cannot reuse a connection
		// with unread data on it, so closing early costs a TLS handshake on the
		// next request. The decoding path drains naturally via ReadAll, so
		// without this a nil v would be quietly slower than a typed one.
		_, err := io.Copy(io.Discard, resp.Body)
		if err != nil {
			return resp, err
		}
		return resp, nil
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, err
	}

	// A non-nil v states that the caller expects a JSON document, and an empty
	// body is not one. JSON can already express "nothing" as null, which
	// decodes cleanly and leaves v at its zero value, so a body with nothing at
	// all in it is a malformed response rather than an empty answer.
	//
	// google/go-github tolerates this by swallowing io.EOF, which suits a
	// client that covers the whole API generically and cannot audit each
	// endpoint. Here every call site targets a known endpoint, so the caller is
	// better served by hearing about it than by receiving a zero-valued struct
	// that looks like a real answer.
	if len(b) == 0 {
		return resp, fmt.Errorf("githubrest: %d response had an empty body, but a JSON document was expected", resp.StatusCode)
	}

	if err := json.Unmarshal(b, v); err != nil {
		return resp, err
	}

	return resp, nil
}

// resolveURL joins path to the client's API base URL, or checks an absolute URL
// against the hosts this client may send credentials to.
//
// The join is textual rather than url.URL.JoinPath, which percent-escapes the
// query separator and so would turn "issues?state=open" into
// "issues%3Fstate=open", silently breaking every filtered and paginated call
// site. go-gh concatenates for the same reason.
func (c *Client) resolveURL(path string) (string, error) {
	if err := checkPathTraversal(path); err != nil {
		return "", err
	}

	if !strings.HasPrefix(path, "https://") && !strings.HasPrefix(path, "http://") {
		return strings.TrimSuffix(c.apiBaseURL.String(), "/") + "/" + strings.TrimPrefix(path, "/"), nil
	}

	parsed, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("githubrest: parsing path %q: %w", path, err)
	}

	if _, ok := c.credentialedHosts[strings.ToLower(parsed.Host)]; !ok {
		return "", fmt.Errorf("githubrest: refusing to send credentials to %q, so if this host belongs to the same deployment declare it with WithCredentialedHost", parsed.Host)
	}

	// A credentialed host reached over plaintext would put the token on the
	// wire, so it is refused even though the host itself is trusted.
	if c.apiBaseURL.Scheme == "https" && parsed.Scheme != "https" {
		return "", fmt.Errorf("githubrest: refusing to request %q over http when the client uses https", path)
	}

	return path, nil
}

func isSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// checkPathTraversal rejects a path containing a ".." segment.
//
// A traversal segment can climb out of the endpoint a caller named, turning
// "repos/OWNER/REPO/../../user" into a request nobody at the call site intended
// and, worse, one that still carries the token. google/go-github rejects the
// same shape. Percent-encoded forms are covered because url.Parse decodes them
// before the check runs. ".." inside a segment, as in "file..txt", is a
// legitimate name and is left alone.
func checkPathTraversal(path string) error {
	parsed, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("githubrest: parsing path %q: %w", path, err)
	}
	if slices.Contains(strings.Split(parsed.Path, "/"), "..") {
		return ErrPathTraversal
	}
	return nil
}
