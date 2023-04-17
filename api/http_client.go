package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/utils"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
)

type tokenGetter interface {
	Token(string) (string, string)
}

type HTTPClientOptions struct {
	AppVersion        string
	CacheTTL          time.Duration
	Config            tokenGetter
	EnableCache       bool
	Log               io.Writer
	LogColorize       bool
	SkipAcceptHeaders bool
}

func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	// Provide invalid host, and token values so gh.HTTPClient will not automatically resolve them.
	// The real host and token are inserted at request time.
	clientOpts := ghAPI.ClientOptions{
		Host:         "none",
		AuthToken:    "none",
		LogIgnoreEnv: true,
	}

	if debugEnabled, debugValue := utils.IsDebugEnabled(); debugEnabled {
		clientOpts.Log = opts.Log
		clientOpts.LogColorize = opts.LogColorize
		clientOpts.LogVerboseHTTP = strings.Contains(debugValue, "api")
	}

	headers := map[string]string{
		userAgent: fmt.Sprintf("GitHub CLI %s", opts.AppVersion),
	}
	if opts.SkipAcceptHeaders {
		headers[accept] = ""
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

	client.Transport = AddASCIISanitizer(client.Transport)

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

// AddAuthToken adds an authentication token header for the host specified by the request.
func AddAuthTokenHeader(rt http.RoundTripper, cfg tokenGetter) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		// If the header is already set in the request, don't overwrite it.
		if req.Header.Get(authorization) == "" {
			hostname := ghinstance.NormalizeHostname(getHost(req))
			if token, _ := cfg.Token(hostname); token != "" {
				req.Header.Set(authorization, fmt.Sprintf("token %s", token))
			}
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

func getHost(r *http.Request) string {
	if r.Host != "" {
		return r.Host
	}
	return r.URL.Hostname()
}
