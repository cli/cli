package ghinstance

import (
	"fmt"
	"strings"
)

const defaultHostname = "github.com"

func Default() string {
	return defaultHostname
}

func IsEnterprise(h string) bool {
	return NormalizeHostname(h) != defaultHostname
}

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
