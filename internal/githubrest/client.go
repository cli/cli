// Package githubrest is a client for GitHub's REST API.
//
// The API is versioned "v3" in its URL paths, but the name is avoided here:
// GitHub now versions the REST API by date through the X-GitHub-Api-Version
// header, so "v3" describes the URL layout rather than the API a caller gets.
// The name also collides confusingly with github.com/shurcooL/githubv4, which
// is a GraphQL query DSL rather than a REST client.
package githubrest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	o "github.com/cli/cli/v2/pkg/option"
)

const (
	authorizationHeader = "Authorization"
	contentTypeHeader   = "Content-Type"
)

// Client sends requests to a single GitHub REST API host.
//
// It holds an already-resolved API base URL: it reads no configuration and has
// no opinion about how a canonical host such as github.example.com becomes an
// API host. That resolution happens once, where the client is constructed.
type Client struct {
	apiBaseURL *url.URL
	token      o.Option[string]
	http       *http.Client

	// credentialedHosts are the hosts this client will send an Authorization
	// header to, lowercased and including any port.
	credentialedHosts map[string]struct{}
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
// It is needed by callers that rewrite a redirect target, such as gh release
// download avoiding legacy Codeload paths, and by callers that halt redirects
// with http.ErrUseLastResponse, such as gh repo delete. Neither is expressible
// through a RequestOption, since a redirect policy belongs to the http.Client
// rather than to a request.
//
// The http.Client is shallow-copied and the policy set on the copy. This is not
// ceremony: NewClient stores the caller's *http.Client, and the CLI hands the
// same one to every command, so setting the field in place would install this
// policy process-wide for unrelated commands. Both call sites named above
// already hand-roll the same copy for the same reason.
func WithCheckRedirect(fn func(*http.Request, []*http.Request) error) ClientOption {
	return func(c *Client) {
		httpClient := &http.Client{}
		if c.http != nil {
			copied := *c.http
			httpClient = &copied
		}
		httpClient.CheckRedirect = fn
		c.http = httpClient
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

	return client, nil
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

// WithSingleRequestToken overrides the client's token for one request.
//
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
// caller must close it.
//
// A non-2xx status yields a nil *Response and an *ErrorResponse. The body is
// closed on that path, because a caller holding no response has nothing to
// close.
func (c *Client) Send(req *http.Request) (*Response, error) {
	resp, err := c.http.Do(req)
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
		defer resp.Body.Close()
		return nil, NewErrorResponse(resp)
	}

	return &Response{Response: resp}, nil
}

// Do issues req, decodes the JSON body into v, and closes the body.
//
// A nil v discards the body without decoding it. Statuses 204 and 205 skip
// decoding, since they carry no content.
//
// Once a response has been received successfully it is always returned, even
// alongside an error: the request succeeded and the status and headers are
// known good, so only body handling failed and the caller should keep them.
// The body of such a response is already closed, since Do closes it
// unconditionally, so only its metadata is readable.
//
// A non-2xx status yields a nil *Response and an *ErrorResponse.
func (c *Client) Do(req *http.Request, v any) (*Response, error) {
	resp, err := c.Send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusResetContent {
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
