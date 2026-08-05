package githubrest

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

const (
	apiVersionHeader = "X-GitHub-Api-Version"
	apiVersionValue  = "2022-11-28"
	userAgentHeader  = "User-Agent"
)

// HTTPClientOptions configures the transport a Client sends through.
type HTTPClientOptions struct {
	AppVersion         string
	CacheTTL           time.Duration
	EnableCache        bool
	InvokingAgent      string
	Log                io.Writer
	LogColorize        bool
	LogVerboseHTTP     bool
	SkipDefaultHeaders bool
	TelemetryDisabler  ghtelemetry.Disabler
}

// NewHTTPClient returns the transport stack a Client sends through: go-gh's
// caching, logging and default-header round trippers, plus the CLI's telemetry
// disabler.
//
// It deliberately does not attach an authentication transport. api.NewHTTPClient
// wraps its client in AddAuthTokenHeader, which resolves a token per request
// from the request's host, so nothing at a call site says which credentials a
// request will carry. Here the token is settled when the Client is constructed,
// which is the point of this package, and a transport that filled in a token for
// an unauthenticated client would quietly undo that.
//
// api.NewHTTPClient survives for GraphQL, which still injects per request. The
// duplication is deliberate and temporary: the eventual shape is one shared
// constructor with REST and GraphQL clients beside it.
func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
	// Host and token are given deliberately invalid values so that go-gh does
	// not resolve real ones from the environment. This client only carries
	// transport concerns; the Client supplies the Authorization header.
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

	clientOpts.Headers = map[string]string{
		userAgentHeader:  ua,
		apiVersionHeader: apiVersionValue,
	}

	if opts.EnableCache {
		clientOpts.EnableCache = opts.EnableCache
		clientOpts.CacheTTL = opts.CacheTTL
	}

	client, err := ghAPI.NewHTTPClient(clientOpts)
	if err != nil {
		return nil, err
	}

	if opts.TelemetryDisabler != nil {
		client.Transport = telemetryDisablerTransport{
			wrappedTransport:  client.Transport,
			telemetryDisabler: opts.TelemetryDisabler,
		}
	}

	return client, nil
}

type telemetryDisablerTransport struct {
	wrappedTransport  http.RoundTripper
	telemetryDisabler ghtelemetry.Disabler
}

func (t telemetryDisablerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	if ghauth.IsEnterprise(host) {
		t.telemetryDisabler.Disable()
	}
	return t.wrappedTransport.RoundTrip(req)
}
