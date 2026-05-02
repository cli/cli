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

type tokenGetter interface {
	ActiveToken(string) (string, string)
}

type APIBaseURLResolver func(hostname string) string

type HTTPClientOptions struct {
	AppVersion         string
	InvokingAgent      string
	APIBaseURL        APIBaseURLResolver
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

	if opts.APIBaseURL != nil {
		client.Transport = RewriteAPIBaseURL(client.Transport, opts.APIBaseURL)
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

	return client, nil
}

func NewCachedHTTPClient(httpClient *http.Client, ttl time.Duration) *http.Client {
	newClient := *httpClient
	newClient.Transport = AddCacheTTLHeader(httpClient.Transport, ttl)
	return &newClient
}

// AddCacheTTLHeader adds an header to the request telling the cache that the request
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

// RewriteAPIBaseURL rewrites requests for a canonical host to a configured API base URL.
// This must run after auth headers are selected so tokens are looked up for the canonical host.
func RewriteAPIBaseURL(rt http.RoundTripper, resolver APIBaseURLResolver) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		hostname := ghauth.NormalizeHostname(getHost(req))
		apiBaseURL := resolver(hostname)
		if apiBaseURL == "" {
			return rt.RoundTrip(req)
		}

		baseURL, err := validateAPIBaseURL(hostname, apiBaseURL)
		if err != nil {
			return nil, err
		}

		rewrittenURL, err := url.Parse(baseURL.JoinPath(req.URL.EscapedPath()).String())
		if err != nil {
			return nil, fmt.Errorf("invalid api_base_url for %s: %w", hostname, err)
		}
		rewrittenURL.RawQuery = req.URL.RawQuery

		rewritten := req.Clone(req.Context())
		rewritten.URL = rewrittenURL
		rewritten.Host = rewrittenURL.Host
		return rt.RoundTrip(rewritten)
	}}
}

func validateAPIBaseURL(hostname, rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid api_base_url for %s: %w", hostname, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid api_base_url for %s: must include scheme and host", hostname)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid api_base_url for %s: must not include query or fragment", hostname)
	}
	return u, nil
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
