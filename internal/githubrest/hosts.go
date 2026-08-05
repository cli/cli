package githubrest

import (
	"fmt"
	"strings"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

const (
	githubHost    = "github.com"
	localhostHost = "github.localhost"
	garageHost    = "garage.github.com"
)

// APIBaseURL maps a canonical GitHub host, such as github.com or
// github.example.com, to the REST API base URL for that deployment.
//
// The CLI had two of these and they disagreed. go-gh's restPrefix normalizes
// the hostname before building the URL and ghinstance.RESTPrefix does not, so
// subdomain.github.com yielded https://api.github.com/ through api.Client.REST
// and https://api.subdomain.github.com/ through any call site that built its
// own URL. Which URL a request got therefore depended on which client built it.
//
// This adopts go-gh's normalizing rule, because api.Client.REST is by far the
// larger population and keeping it unchanged keeps the api package's tests as a
// regression suite. Call sites that previously built their own URL through
// ghinstance.RESTPrefix therefore move, but only for hosts that normalize to
// something else, which is subdomains of github.com, github.localhost and
// ghe.com tenancies.
//
// garage.github.com is checked before normalization because it would otherwise
// normalize to github.com and lose its /api/v3/ prefix.
func APIBaseURL(hostname string) string {
	if strings.EqualFold(hostname, garageHost) {
		return fmt.Sprintf("https://%s/api/v3/", hostname)
	}

	hostname = ghauth.NormalizeHostname(hostname)

	if ghauth.IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/api/v3/", hostname)
	}

	// github.localhost is served over plaintext, being a local development
	// instance, so this is the one host where the API base URL is not https.
	if strings.EqualFold(hostname, localhostHost) {
		return fmt.Sprintf("http://api.%s/", hostname)
	}

	return fmt.Sprintf("https://api.%s/", hostname)
}

// UploadHost returns the host that serves the REST upload endpoint for a
// deployment, which is only a different host on github.com.
//
// It exists so that a caller can hand the result to WithCredentialedHost
// without having to know whether this deployment splits uploads out.
func UploadHost(hostname string) string {
	if strings.EqualFold(hostname, garageHost) {
		return hostname
	}

	normalized := ghauth.NormalizeHostname(hostname)
	if ghauth.IsEnterprise(normalized) {
		return normalized
	}
	if strings.EqualFold(normalized, localhostHost) {
		return "uploads." + normalized
	}

	return "uploads." + normalized
}
