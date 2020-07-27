package ghinstance

import (
	"fmt"
	"strings"
)

const defaultHostname = "github.com"

// Default returns the host name of the default GitHub instance
func Default() string {
	return defaultHostname
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
