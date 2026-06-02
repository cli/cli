package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/utils"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type tokenGetter interface {
	ActiveToken(string) (string, string)
}

type HTTPClientOptions struct {
	AppVersion         string
	InvokingAgent      string
	CacheTTL           time.Duration
	Config             tokenGetter
	EnableCache        bool
	Log                io.Writer
	LogColorize        bool
	LogVerboseHTTP     bool
	SkipDefaultHeaders bool
	TelemetryDisabler  ghtelemetry.Disabler
}

func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	// Provide invalid host, and token values so gh.HTTPClient will not automatically resolve them.
	// The real host and token are inserted at request time.
	clientOpts := ghAPI.ClientOptions{
		Host:               "none",
		AuthToken:          "none",
		LogIgnoreEnv:       true,
		SkipDefaultHeaders: opts.SkipDefaultHeaders,
	}

	debugEnabled, debugValue := utils.IsDebugEnabled()
	if strings.Contains(debugValue, "api") {
		opts.LogVerboseHTTP = true
	}

	if opts.LogVerboseHTTP || debugEnabled {
		clientOpts.Log = opts.Log
		clientOpts.LogColorize = opts.LogColorize
		clientOpts.LogVerboseHTTP = opts.LogVerboseHTTP
	}

	ua := fmt.Sprintf("GitHub CLI %s", opts.AppVersion)
	if opts.InvokingAgent != "" {
		ua = fmt.Sprintf("%s Agent/%s", ua, opts.InvokingAgent)
	}

	headers := map[string]string{
		userAgent:  ua,
		apiVersion: apiVersionValue,
	}
	clientOpts.Headers = headers

	if opts.EnableCache {
		clientOpts.EnableCache = opts.EnableCache
		clientOpts.CacheTTL = opts.CacheTTL
	}

	client, err := ghAPI.NewHTTPClient(clientOpts)
	if err != nil {
		return nil, err
	}

	if opts.Config != nil {
		client.Transport = AddAuthTokenHeader(client.Transport, opts.Config)
	}

	if opts.TelemetryDisabler != nil {
		client.Transport = telemetryDisablerTransport{
			wrappedTransport:  client.Transport,
			telemetryDisabler: opts.TelemetryDisabler,
		}
	}

	// Consulted here, in the single client constructor, so callers that build
	// their own options (e.g. `gh api`) can't bypass it.
	if ghenv.ReadOnly() {
		client.Transport = ReadOnlyMiddleware(client.Transport)
	}

	return client, nil
}

// ExternalHTTPClientOptions holds options for creating an external HTTP client.
type ExternalHTTPClientOptions struct {
	AppVersion  string
	Log         io.Writer
	LogColorize bool
	Transport   http.RoundTripper
}

// NewExternalHTTPClient creates an HTTP client for talking to non-GitHub hosts.
// It includes debug logging and a User-Agent header but does not attach any
// authentication tokens or GitHub-specific headers.
func NewExternalHTTPClient(opts ExternalHTTPClientOptions) (*http.Client, error) {
	clientOpts := ghAPI.ClientOptions{
		Host:               "none",
		AuthToken:          "none",
		LogIgnoreEnv:       true,
		SkipDefaultHeaders: true,
		Transport:          opts.Transport,
	}

	debugEnabled, debugValue := utils.IsDebugEnabled()
	logVerboseHTTP := false
	if strings.Contains(debugValue, "api") {
		logVerboseHTTP = true
	}

	if logVerboseHTTP || debugEnabled {
		clientOpts.Log = opts.Log
		clientOpts.LogColorize = opts.LogColorize
		clientOpts.LogVerboseHTTP = logVerboseHTTP
	}

	clientOpts.Headers = map[string]string{
		userAgent: fmt.Sprintf("GitHub CLI %s", opts.AppVersion),
	}

	client, err := ghAPI.NewHTTPClient(clientOpts)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func NewCachedHTTPClient(httpClient *http.Client, ttl time.Duration) *http.Client {
	newClient := *httpClient
	newClient.Transport = AddCacheTTLHeader(httpClient.Transport, ttl)
	return &newClient
}

// AddCacheTTLHeader adds a header to the request telling the cache that the request
// should be cached for a specified amount of time.
func AddCacheTTLHeader(rt http.RoundTripper, ttl time.Duration) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(cacheTTL) == "" {
			req.Header.Set(cacheTTL, ttl.String())
		}
		return rt.RoundTrip(req)
	}}
}

// AddAuthTokenHeader adds an authentication token header for the host specified by the request.
func AddAuthTokenHeader(rt http.RoundTripper, cfg tokenGetter) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(authorization) == "" {
			var redirectHostnameChange bool
			if req.Response != nil && req.Response.Request != nil {
				redirectHostnameChange = getHost(req) != getHost(req.Response.Request)
			}
			// Only set header if an initial request or redirect request to the same host as the initial request.
			// If the host has changed during a redirect do not add the authentication token header.
			if !redirectHostnameChange {
				hostname := ghauth.NormalizeHostname(getHost(req))
				if token, _ := cfg.ActiveToken(hostname); token != "" {
					req.Header.Set(authorization, fmt.Sprintf("token %s", token))
				}
			}
		}
		return rt.RoundTrip(req)
	}}
}

var ErrReadOnly = errors.New("gh is in read-only mode (GH_READ_ONLY): this operation would modify data and was blocked")

func ReadOnlyMiddleware(rt http.RoundTripper) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		if err := checkReadOnly(req); err != nil {
			return nil, err
		}
		return rt.RoundTrip(req)
	}}
}

func checkReadOnly(req *http.Request) error {
	// HTTP methods are case-sensitive per the spec but GitHub treats them
	// case-insensitively, and `gh api -X get` reaches here as "get", so
	// normalize before comparing to avoid blocking a lowercase read.
	switch strings.ToUpper(req.Method) {
	case http.MethodGet, http.MethodHead:
		return nil
	}

	if isGraphQLEndpoint(req.URL.Path) {
		isMutation, err := requestIsGraphQLMutation(req)
		if err != nil {
			// Fail closed: if we cannot determine the operation type, block it.
			return ErrReadOnly
		}
		if !isMutation {
			return nil
		}
	}

	return ErrReadOnly
}

// "/graphql" on github.com, "/api/graphql" on GHES
func isGraphQLEndpoint(path string) bool {
	return path == "/graphql" || path == "/api/graphql"
}

// Reads and restores req.Body, so the request stays sendable afterward.
func requestIsGraphQLMutation(req *http.Request) (bool, error) {
	if req.Body == nil {
		return false, nil
	}

	defer req.Body.Close()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return false, err
	}

	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	var payload struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, err
	}

	return containsTopLevelMutation(payload.Query), nil
}

// containsTopLevelMutation detects a mutation *operation* — not merely the word
// "mutation", which may also be a field or operation name.
func containsTopLevelMutation(query string) bool {
	depth := 0
	// expectKeyword is true at the start of the document and after a top-level
	// definition closes, i.e. exactly where an operation/fragment keyword would
	// appear next.
	expectKeyword := true

	for i := 0; i < len(query); {
		c := query[i]

		switch {
		case c == '#':
			// Comment runs to end of line.
			for i < len(query) && query[i] != '\n' {
				i++
			}
		case c == '"':
			i = skipGraphQLString(query, i)
		case c == '{':
			depth++
			expectKeyword = false
			i++
		case c == '}':
			if depth > 0 {
				depth--
			}
			if depth == 0 {
				expectKeyword = true
			}
			i++
		case isNameByte(c):
			start := i
			for i < len(query) && isNameByte(query[i]) {
				i++
			}
			if depth == 0 && expectKeyword {
				if query[start:i] == "mutation" {
					return true
				}
				// The definition keyword has been seen; do not treat later
				// top-level identifiers (e.g. the operation name) as keywords.
				expectKeyword = false
			}
		default:
			i++
		}
	}

	return false
}

// skipGraphQLString returns the index just past the string literal at query[i].
func skipGraphQLString(query string, i int) int {
	if strings.HasPrefix(query[i:], `"""`) {
		i += 3
		for i < len(query) {
			if strings.HasPrefix(query[i:], `"""`) {
				return i + 3
			}
			i++
		}
		return i
	}

	i++ // opening quote
	for i < len(query) {
		switch query[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1
		default:
			i++
		}
	}
	return i
}

func isNameByte(c byte) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

// ExtractHeader extracts a named header from any response received by this client and,
// if non-blank, saves it to dest.
func ExtractHeader(name string, dest *string) func(http.RoundTripper) http.RoundTripper {
	return func(tr http.RoundTripper) http.RoundTripper {
		return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
			res, err := tr.RoundTrip(req)
			if err == nil {
				if value := res.Header.Get(name); value != "" {
					*dest = value
				}
			}
			return res, err
		}}
	}
}

type funcTripper struct {
	roundTrip func(*http.Request) (*http.Response, error)
}

func (tr funcTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return tr.roundTrip(req)
}

func getHost(r *http.Request) string {
	if r.Host != "" {
		return r.Host
	}
	return r.URL.Host
}

type telemetryDisablerTransport struct {
	wrappedTransport  http.RoundTripper
	telemetryDisabler ghtelemetry.Disabler
}

func (t telemetryDisablerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if ghauth.IsEnterprise(getHost(req)) {
		t.telemetryDisabler.Disable()
	}
	return t.wrappedTransport.RoundTrip(req)
}
