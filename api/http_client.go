package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/utils"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type config interface {
	ActiveToken(string) (string, string)
	HostForAPIHost(string) (string, bool)
}

type HTTPClientOptions struct {
	AppVersion          string
	InvokingAgent       string
	CacheTTL            time.Duration
	Config              config
	EnableCache         bool
	Log                 io.Writer
	LogColorize         bool
	LogVerboseHTTP      bool
	APIRequestTelemetry ghtelemetry.APIRequestTelemetry
	SkipDefaultHeaders  bool
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
	clientOpts.Transport = requestIDTransport{
		wrappedTransport: http.DefaultTransport,
		recorder:         opts.APIRequestTelemetry,
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

	client.Transport = telemetryDisablerTransport{
		wrappedTransport:  client.Transport,
		telemetryDisabler: opts.APIRequestTelemetry,
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
func AddAuthTokenHeader(rt http.RoundTripper, cfg config) http.RoundTripper {
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
		token, _ := cfg.ActiveToken(hostnameInRequest)
		if token == "" {
			// The request may be aimed at a host's api_host, which gh is
			// not logged in to and so has no token of its own. Fall back
			// to the token of the host it stands in for. This only ever
			// adds a token where there would have been none, so hosts we
			// already authenticate keep resolving exactly as before.
			if canonicalHost, ok := cfg.HostForAPIHost(hostnameInRequest); ok {
				token, _ = cfg.ActiveToken(canonicalHost)
			}
		}
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

type requestIDTransport struct {
	wrappedTransport http.RoundTripper
	recorder         ghtelemetry.APIRequestRecorder
}

func (t requestIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.wrappedTransport.RoundTrip(req)
	if err == nil && res != nil {
		if requestID := strings.TrimSpace(res.Header.Get("X-GitHub-Request-Id")); requestID != "" {
			t.recorder.RecordAPIRequest(ghtelemetry.APIRequest{RequestID: requestID})
		}
	}
	return res, err
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
