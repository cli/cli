package api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/utils"
	"github.com/cli/go-gh"
	ghAPI "github.com/cli/go-gh/pkg/api"
)

type configGetter interface {
	Get(string, string) (string, error)
	AuthToken(string) (string, string)
}

type HTTPClientOptions struct {
	AppVersion        string
	CacheTTL          time.Duration
	Config            configGetter
	EnableCache       bool
	Log               io.Writer
	SkipAcceptHeaders bool
}

func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	// Provide invalid host, and token values so gh.HTTPClient will not automatically resolve them.
	// The real host and token are inserted at request time.
	clientOpts := ghAPI.ClientOptions{Host: "none", AuthToken: "none"}

	if debugEnabled, _ := utils.IsDebugEnabled(); debugEnabled {
		clientOpts.Log = opts.Log
	}

	headers := map[string]string{
		"User-Agent": fmt.Sprintf("GitHub CLI %s", opts.AppVersion),
	}
	if opts.SkipAcceptHeaders {
		headers["Accept"] = ""
	}
	clientOpts.Headers = headers

	if opts.EnableCache {
		clientOpts.EnableCache = opts.EnableCache
		clientOpts.CacheTTL = opts.CacheTTL
	}

	client, err := gh.HTTPClient(&clientOpts)
	if err != nil {
		return nil, err
	}

	if opts.Config != nil {
		client.Transport = AddAuthTokenHeader(client.Transport, opts.Config)
	}

	return client, nil
}

func NewCachedHTTPClient(httpClient *http.Client, ttl time.Duration) *http.Client {
	httpClient.Transport = AddCacheTTLHeader(httpClient.Transport, ttl)
	return httpClient
}

// AddCacheTTLHeader adds an header to the request telling the cache that the request
// should be cached for a specified amount of time.
func AddCacheTTLHeader(rt http.RoundTripper, ttl time.Duration) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		req.Header.Set("X-GH-CACHE-TTL", ttl.String())
		return rt.RoundTrip(req)
	}}
}

// AddAuthToken adds an authentication token header for the host specified by the request.
func AddAuthTokenHeader(rt http.RoundTripper, cfg configGetter) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		hostname := ghinstance.NormalizeHostname(getHost(req))
		if token, _ := cfg.AuthToken(hostname); token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
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
