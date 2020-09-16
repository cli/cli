package ghinstance

import (
	"fmt"
	"strings"
)

const defaultHostname = "github.com"

var hostnameOverride string

// Default returns the host name of the default GitHub instance
func Default() string {
	return defaultHostname
}

// OverridableDefault is like Default, except it is overridable by the GH_HOST environment variable
func OverridableDefault() string {
	if hostnameOverride != "" {
		return hostnameOverride
	}
	return defaultHostname
}

// OverrideDefault overrides the value returned from OverridableDefault. This should only ever be
// called from the main runtime path, not tests.
func OverrideDefault(newhost string) {
	hostnameOverride = newhost
}

// IsEnterprise reports whether a non-normalized host name looks like a GHE instance
func IsEnterprise(h string) bool {
	return NormalizeHostname(h) != defaultHostname
}

// NormalizeHostname returns the canonical host name of a GitHub instance
func NormalizeHostname(h string) string {
	hostname := strings.ToLower(h)
	if strings.HasSuffix(hostname, "."+defaultHostname) {
		return defaultHostname
	}
	return hostname
}

func GraphQLEndpoint(hostname string) string {
	if IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/api/graphql", hostname)
	}
	return "https://api.github.com/graphql"
}

func RESTPrefix(hostname string) string {
	if IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/api/v3/", hostname)
	}
	return "https://api.github.com/"
}

func GistPrefix(hostname string) string {
	if IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/gist/", hostname)
	}
	return fmt.Sprintf("https://gist.%s/", hostname)
}
