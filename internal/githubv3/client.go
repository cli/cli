// Package githubv3 is a client for GitHub's REST (v3) API. It is unrelated to
// github.com/shurcooL/githubv4, which is a GraphQL query DSL rather than a client.
package githubv3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const authorizationHeader = "Authorization"

// Client sends requests to a single GitHub REST API host.
//
// It holds an already-resolved API base URL: it reads no configuration and has
// no opinion about how a canonical host such as github.example.com becomes an
// API host. That resolution happens once, where the client is constructed.
//
// A token is optional, and its absence means "let the transport supply
// credentials", never "send an unauthenticated request". See WithClientToken.
type Client struct {
	apiBaseURL *url.URL
	token      *string
	http       *http.Client
}

// ClientOption configures a Client at construction.
type ClientOption func(*Client)

// WithClientToken sets the token the client authenticates with, and is the only
// way to give a client a token.
//
// It always sets the Authorization header, including when t is empty, so the
// client authenticates as exactly the given token and nothing else. Omitting
// this option does not mean "unauthenticated": it means the header is left
// unset, and whatever credentials the transport supplies are used instead. The
// package deliberately offers no way to express "send no credentials", because
// no caller needs it and the transport would override it anyway.
//
// Callers such as gh auth status, which check one specific user's token rather
// than the host's active one, must pass this option even when the token they
// hold is empty. Without it, api.AddAuthTokenHeader steps in whenever
// Authorization is unset and authenticates as the *active* user, so the command
// would report on the wrong credentials. That is why an empty t still sets the
// header: the value is rendered as "token ", which is non-empty, and both
// AddAuthTokenHeader and go-gh's header round tripper stand down only when
// Authorization is non-empty. A header set to "" would not stand them down.
//
// Forgetting this option is therefore a silent wrong-answer hazard rather than
// an error. Containing it here is deliberate; fixing the transport so that it
// cannot override a client's intent is tracked separately.
func WithClientToken(t string) ClientOption {
	return func(c *Client) {
		c.token = &t
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
// httpClient.
//
// apiBaseURL is a resolved, absolute API base URL such as
// https://api.github.com/ or https://github.example.com/api/v3/, not a
// canonical host. It is rejected here rather than per request, so a Client
// either resolves relative paths correctly or does not exist.
//
// This takes a string and returns an error where the migration plan's Task 1
// names apiBaseURL *url.URL and no error. The invariant is worth more than
// saving a caller a url.Parse, so a caller holding a parsed URL passes
// u.String().
//
// Without WithClientToken the client sets no Authorization header, which means
// the transport supplies credentials rather than that requests carry none.
func NewClient(apiBaseURL string, httpClient *http.Client, opts ...ClientOption) (*Client, error) {
	if apiBaseURL == "" {
		return nil, errors.New("githubv3: apiBaseURL is required")
	}

	parsed, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("githubv3: parsing apiBaseURL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("githubv3: apiBaseURL %q is not absolute", apiBaseURL)
	}

	client := &Client{
		apiBaseURL: parsed,
		http:       httpClient,
	}

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

// WithToken overrides the client's token for a single request.
//
// It always sets the Authorization header, including when token is empty,
// mirroring WithClientToken: the request authenticates as exactly this token.
// Omitting it leaves the client's own token in place. There is deliberately no
// way to express "send no credentials" for the reasons given at
// WithClientToken.
func WithToken(token string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set(authorizationHeader, authorizationValue(token))
	}
}

// authorizationValue renders a token as an Authorization header value.
//
// An empty token still yields a non-empty value, which is what makes
// api.AddAuthTokenHeader and go-gh's header round tripper stand down: both
// check only whether Authorization is non-empty.
func authorizationValue(token string) string {
	return fmt.Sprintf("token %s", token)
}

// NewRequest builds a request against the client's host.
//
// path may be a path relative to the client's API base URL, or an absolute
// URL. Absolute URLs are required by callers that follow server-supplied URLs,
// such as release asset uploads.
//
// The Authorization header is set from the client's token, when it has one,
// before opts are applied, so an option can override it. A client with no token
// leaves the header unset for the transport to fill in.
//
// No other headers are set here. User-Agent, X-GitHub-Api-Version and, unless
// they are skipped, Accept, Content-Type and Time-Zone are applied by go-gh's
// header round tripper at RoundTrip time, and it skips any header already
// present on the request. An option therefore wins by being set earlier, not
// later, and setting defaults here would double-set them.
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader, opts ...RequestOption) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.resolveURL(path), body)
	if err != nil {
		return nil, err
	}

	if c.token != nil {
		req.Header.Set(authorizationHeader, authorizationValue(*c.token))
	}

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
		// Drained rather than ignored so the connection can be reused.
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

	// An empty body reaches json.Unmarshal and fails with "unexpected end of
	// JSON input". That is deliberate: it is what api.Client.REST and go-gh do
	// today, so tolerating it the way google/go-github does would silently
	// change behaviour at every existing call site.
	if err := json.Unmarshal(b, v); err != nil {
		return resp, err
	}

	return resp, nil
}

// resolveURL joins path to the client's API base URL, passing absolute URLs
// through untouched.
//
// The join is textual rather than url.URL.JoinPath, which percent-escapes the
// query separator and so would turn "issues?state=open" into
// "issues%3Fstate=open", silently breaking every filtered and paginated call
// site. go-gh concatenates for the same reason.
func (c *Client) resolveURL(path string) string {
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		return path
	}
	return strings.TrimSuffix(c.apiBaseURL.String(), "/") + "/" + strings.TrimPrefix(path, "/")
}

func isSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}
