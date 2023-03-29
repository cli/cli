package ghinstance

import (
	"errors"
	"fmt"
	"strings"
)

const defaultHostname = "github.com"

// localhost is the domain name of a local GitHub instance
const localhost = "github.localhost"

// Default returns the host name of the default GitHub instance
func Default() string {
	return defaultHostname
}

// IsEnterprise reports whether a non-normalized host name looks like a GHE instance
func IsEnterprise(h string) bool {
	normalizedHostName := NormalizeHostname(h)
	return normalizedHostName != defaultHostname && normalizedHostName != localhost
}

func isGarage(h string) bool {
	return strings.EqualFold(h, "garage.github.com")
}

// NormalizeHostname returns the canonical host name of a GitHub instance
func NormalizeHostname(h string) string {
	hostname := strings.ToLower(h)
	if strings.HasSuffix(hostname, "."+defaultHostname) {
		return defaultHostname
	}

	if strings.HasSuffix(hostname, "."+localhost) {
		return localhost
	}

	return hostname
}

func HostnameValidator(hostname string) error {
	if len(strings.TrimSpace(hostname)) < 1 {
		return errors.New("a value is required")
	}
	if strings.ContainsRune(hostname, '/') || strings.ContainsRune(hostname, ':') {
		return errors.New("invalid hostname")
	}
	return nil
}

func GraphQLEndpoint(hostname string) string {
	if isGarage(hostname) {
		return fmt.Sprintf("https://%s/api/graphql", hostname)
	}
	if IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/api/graphql", hostname)
	}
	if strings.EqualFold(hostname, localhost) {
		return fmt.Sprintf("http://api.%s/graphql", hostname)
	}
	return fmt.Sprintf("https://api.%s/graphql", hostname)
}

func RESTPrefix(hostname string) string {
	if isGarage(hostname) {
		return fmt.Sprintf("https://%s/api/v3/", hostname)
	}
	if IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/api/v3/", hostname)
	}
	if strings.EqualFold(hostname, localhost) {
		return fmt.Sprintf("http://api.%s/", hostname)
	}
	return fmt.Sprintf("https://api.%s/", hostname)
}

func GistPrefix(hostname string) string {
	prefix := "https://"

	if strings.EqualFold(hostname, localhost) {
		prefix = "http://"
	}

	return prefix + GistHost(hostname)
}

func GistHost(hostname string) string {
	if isGarage(hostname) {
		return fmt.Sprintf("%s/gist/", hostname)
	}
	if IsEnterprise(hostname) {
		return fmt.Sprintf("%s/gist/", hostname)
	}
	if strings.EqualFold(hostname, localhost) {
		return fmt.Sprintf("%s/gist/", hostname)
	}
	return fmt.Sprintf("gist.%s/", hostname)
}

func HostPrefix(hostname string) string {
	if strings.EqualFold(hostname, localhost) {
		return fmt.Sprintf("http://%s/", hostname)
	}
	return fmt.Sprintf("https://%s/", hostname)
}
