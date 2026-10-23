package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/pkg/auth"
	"github.com/cli/cli/v2/utils"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

// config is the token source used to authenticate requests. It uses primitive types so packages outside
// github.com/cli/cli/v2 can implement it. It is the public, backward-compatible seam and does not support refreshing an
// expiring token.
type config interface {
	ActiveToken(string) (string, string)
	HostForAPIHost(string) (string, bool)
}

// refreshingConfig is a refresh-aware token source. It can renew a near-expiry token before a request and
// is implemented by gh's own configuration. It references the public auth credential types rather than primitive
// strings so the resolved token and its refresh metadata travel together.
type refreshingConfig interface {
	ActiveTokenWithRefresh(string) (auth.Credential, auth.RefreshStatus, error)
	HostForAPIHost(string) (string, bool)
}

type HTTPClientOptions struct {
	AppVersion    string
	InvokingAgent string
	CacheTTL      time.Duration
	// Config is the primitive, backward-compatible token source for authenticating requests. Provide either Config or
	// RefreshingConfig, never both; NewHTTPClient returns an error if both are set. A token from Config is used as-is,
	// with no refresh.
	Config config
	// RefreshingConfig is the refresh-aware token source gh supplies for its own clients. Provide either Config or
	// RefreshingConfig, never both; NewHTTPClient returns an error if both are set. When set, a near-expiry
	// token is refreshed on demand before each request.
	RefreshingConfig refreshingConfig
	EnableCache      bool
	Log              io.Writer
	LogColorize      bool
	LogVerboseHTTP   bool
	// LogHeadlineOnly keeps logging non-verbose even when GH_DEBUG requests api tracing, so only httpretty's head
	// lines (timing and URL) are emitted, not request/response headers and bodies. It is used by the token refresher
	// client so a refresh triggered mid request does not dump a full trace into the middle of the user's request log.
	LogHeadlineOnly    bool
	SkipDefaultHeaders bool
	TelemetryDisabler  ghtelemetry.Disabler
}

func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	if opts.Config != nil && opts.RefreshingConfig != nil {
		return nil, errors.New("provide either Config or RefreshingConfig, not both")
	}

	// Provide invalid host, and token values so gh.HTTPClient will not automatically resolve them.
	// The real host and token are inserted at request time.
	clientOpts := ghAPI.ClientOptions{
		Host:               "none",
		AuthToken:          "none",
		LogIgnoreEnv:       true,
		SkipDefaultHeaders: opts.SkipDefaultHeaders,
	}

	debugEnabled, debugValue := utils.IsDebugEnabled()
	if strings.Contains(debugValue, "api") && !opts.LogHeadlineOnly {
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

	if opts.RefreshingConfig != nil {
		client.Transport = AddAuthTokenHeaderWithRefresh(client.Transport, opts.RefreshingConfig)
	} else if opts.Config != nil {
		client.Transport = AddAuthTokenHeader(client.Transport, opts.Config)
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
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(cacheTTL) == "" {
			req.Header.Set(cacheTTL, ttl.String())
		}
		return rt.RoundTrip(req)
	}}
}

// AddAuthTokenHeader adds an authentication token header for the host specified by the request, using the primitive
// token source. It is the backward-compatible entry point retained for external callers of this package; gh's own code
// authenticates through the refresh-aware AddAuthTokenHeaderWithRefresh.
func AddAuthTokenHeader(rt http.RoundTripper, cfg config) http.RoundTripper {
	return addAuthHeaderRoundTripper(rt, func(host string) string {
		token, _ := cfg.ActiveToken(host)
		if token == "" {
			// The request may be aimed at a host's api_host, which gh is
			// not logged in to and so has no token of its own. Fall back
			// to the token of the host it stands in for. This only ever
			// adds a token where there would have been none, so hosts we
			// already authenticate keep resolving exactly as before.
			if canonicalHost, ok := cfg.HostForAPIHost(host); ok {
				token, _ = cfg.ActiveToken(canonicalHost)
			}
		}
		return token
	})
}

// AddAuthTokenHeaderWithRefresh is the refresh-aware counterpart of AddAuthTokenHeader. It renews a near-expiry
// token before attaching it, and is the attach helper gh's own code uses so its requests go through the
// refresh path rather than the backward-compatible primitive seam.
func AddAuthTokenHeaderWithRefresh(rt http.RoundTripper, cfg refreshingConfig) http.RoundTripper {
	return addAuthHeaderRoundTripper(rt, func(host string) string {
		// Refreshing is best effort because it happens before actual expiry, and expiry checks can be unreliable due
		// to clock skew. Continue with the returned token and let the API determine whether it is valid.
		cred, _, _ := cfg.ActiveTokenWithRefresh(host)
		token := cred.Token
		if token == "" {
			// The request may be aimed at a host's api_host, which gh is
			// not logged in to and so has no token of its own. Fall back
			// to the token of the host it stands in for. This only ever
			// adds a token where there would have been none, so hosts we
			// already authenticate keep resolving exactly as before.
			if canonicalHost, ok := cfg.HostForAPIHost(host); ok {
				cred, _, _ = cfg.ActiveTokenWithRefresh(canonicalHost)
				token = cred.Token
			}
		}
		return token
	})
}

// addAuthHeaderRoundTripper wraps rt so each request receives the token returned by tokenForHost for its (normalized)
// host, unless the Authorization header is already set or the request is a redirect to a different host. tokenForHost
// encapsulates how the token is resolved, which is the only difference between the primitive and refresh-aware paths.
func addAuthHeaderRoundTripper(rt http.RoundTripper, tokenForHost func(host string) string) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(authorization) != "" {
			return rt.RoundTrip(req)
		}

		var redirectHostnameChange bool
		if req.Response != nil && req.Response.Request != nil {
			redirectHostnameChange = getHostname(req) != getHostname(req.Response.Request)
		}

		// Only set header if an initial request or redirect request to the same host as the initial request.
		// If the host has changed during a redirect do not add the authentication token header.
		if redirectHostnameChange {
			return rt.RoundTrip(req)
		}

		hostnameInRequest := ghauth.NormalizeHostname(getHostname(req))
		token := tokenForHost(hostnameInRequest)
		if token != "" {
			req.Header.Set(authorization, fmt.Sprintf("token %s", token))
		}

		return rt.RoundTrip(req)
	}}
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

// getHostname returns the hostname the request is addressed to, preferring an
// explicit Host override and falling back to the URL host. Any port in the host
// is stripped, so callers compare on the hostname alone.
func getHostname(r *http.Request) string {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	return (&url.URL{Host: host}).Hostname()
}

type telemetryDisablerTransport struct {
	wrappedTransport  http.RoundTripper
	telemetryDisabler ghtelemetry.Disabler
}

func (t telemetryDisablerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// If the request is aimed at an enterprise host, disable telemetry.
	// Currently, requests that are sent to configured api_hosts will be treated as enterprise hosts,
	// even though they may be GHEC with Data Residency.
	if ghauth.IsEnterprise(getHostname(req)) {
		t.telemetryDisabler.Disable()
	}
	return t.wrappedTransport.RoundTrip(req)
}
