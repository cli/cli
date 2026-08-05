package api

import (
	"fmt"
	"io"
	"net/http"
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
		client.Transport = AddTokenCarrier(client.Transport, opts.Config)
	}

	if opts.TelemetryDisabler != nil {
		client.Transport = telemetryDisablerTransport{
			wrappedTransport:  client.Transport,
			telemetryDisabler: opts.TelemetryDisabler,
		}
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
	return &funcTripper{inner: rt, roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(cacheTTL) == "" {
			req.Header.Set(cacheTTL, ttl.String())
		}
		return rt.RoundTrip(req)
	}}
}

// tokenCarrier is a RoundTripper that resolves a token for a host but never
// puts one on a request.
//
// It replaces AddAuthTokenHeader, which injected Authorization at RoundTrip
// time. Injecting there meant a client could not state that a request is
// deliberately unauthenticated: githubrest.WithoutToken sets no header, and the
// transport would then supply the active user's token anyway, so the option
// could only express intent. Carrying the token source without applying it
// moves the decision to whoever builds the request, which is where the
// knowledge is.
//
// The token source lives on the transport chain rather than on api.Client
// because api.Client is host-agnostic and config-free by construction, and 176
// non-test call sites build one from an *http.Client alone. Threading a config
// to all of them is the real migration; this keeps the spike honest about where
// the token comes from without doing it.
type tokenCarrier struct {
	inner http.RoundTripper
	cfg   tokenGetter
}

func (t tokenCarrier) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.inner.RoundTrip(req)
}

// Unwrap exposes the wrapped transport so that tokenForHost can walk a chain
// that was built by wrapping this one.
func (t tokenCarrier) Unwrap() http.RoundTripper { return t.inner }

// activeToken returns the token configured for an already-normalized hostname.
func (t tokenCarrier) activeToken(hostname string) string {
	token, _ := t.cfg.ActiveToken(hostname)
	return token
}

// AddTokenCarrier returns a transport that can resolve the token for a host
// without adding an Authorization header to any request.
func AddTokenCarrier(rt http.RoundTripper, cfg tokenGetter) http.RoundTripper {
	return tokenCarrier{inner: rt, cfg: cfg}
}

// tokenForHost walks a transport chain looking for a tokenCarrier and returns
// the token it resolves for hostname, or "" when the chain carries no token
// source. hostname is normalized here, matching the lookup AddAuthTokenHeader
// used to do at RoundTrip time.
func tokenForHost(rt http.RoundTripper, hostname string) string {
	normalized := ghauth.NormalizeHostname(hostname)
	for rt != nil {
		if carrier, ok := rt.(tokenCarrier); ok {
			return carrier.activeToken(normalized)
		}
		unwrapper, ok := rt.(interface{ Unwrap() http.RoundTripper })
		if !ok {
			return ""
		}
		rt = unwrapper.Unwrap()
	}
	return ""
}

// ExtractHeader extracts a named header from any response received by this client and,
// if non-blank, saves it to dest.
func ExtractHeader(name string, dest *string) func(http.RoundTripper) http.RoundTripper {
	return func(tr http.RoundTripper) http.RoundTripper {
		return &funcTripper{inner: tr, roundTrip: func(req *http.Request) (*http.Response, error) {
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
	inner     http.RoundTripper
	roundTrip func(*http.Request) (*http.Response, error)
}

func (tr funcTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return tr.roundTrip(req)
}

// Unwrap exposes the wrapped transport so that a chain built from these can be
// walked, which is how tokenForHost finds the token carrier.
func (tr funcTripper) Unwrap() http.RoundTripper { return tr.inner }

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

// Unwrap exposes the wrapped transport so that a chain built from these can be
// walked, which is how tokenForHost finds the token carrier.
func (t telemetryDisablerTransport) Unwrap() http.RoundTripper { return t.wrappedTransport }
