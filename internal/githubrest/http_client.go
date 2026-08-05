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

// HTTPClientOptions configures NewHTTPClient.
//
// It is deliberately the same shape as api.HTTPClientOptions minus Config,
// since there is no token to resolve here.
type HTTPClientOptions struct {
	AppVersion         string
	InvokingAgent      string
	CacheTTL           time.Duration
	EnableCache        bool
	Log                io.Writer
	LogColorize        bool
	LogVerboseHTTP     bool
	SkipDefaultHeaders bool
	TelemetryDisabler  ghtelemetry.Disabler
}

// NewHTTPClient returns an *http.Client suitable for handing to NewClient.
//
// It is a copy of api.NewHTTPClient with the AddAuthTokenHeader transport
// removed. That transport resolves a token from the request URL and injects an
// Authorization header, which duplicates what NewClient's AuthStrategy already
// does and, more importantly, means WithoutToken over such a client cannot
// actually guarantee an anonymous request. Over a client from here it can.
//
// It is a copy rather than a shared implementation because api.NewHTTPClient
// still has to build the token-injecting client for the GraphQL half of
// api.Client, which is not part of this package's remit.
//
// The go-gh client is built with Host and AuthToken both set to the sentinel
// "none". That is not just to stop go-gh resolving real values: go-gh's header
// round tripper turns a non-empty AuthToken into an Authorization header, and
// only suppresses it because the sentinel host never matches the request host.
// Setting Host to something real here would leak "token none" onto requests.
func NewHTTPClient(opts HTTPClientOptions) (*http.Client, error) {
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
