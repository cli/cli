package ghinstance

import (
	"errors"
	"fmt"
	"strings"
)

// DefaultHostname is the domain name of the default GitHub instance.
const defaultHostname = "github.com"

// Localhost is the domain name of a local GitHub instance.
const localhost = "github.localhost"

// TenancyHost is the domain name of a tenancy GitHub instance.
const tenancyHost = "ghe.com"

// Default returns the host name of the default GitHub instance.
func Default() string {
	return defaultHostname
}

// IsEnterprise reports whether a non-normalized host name looks like a GHE instance.
func IsEnterprise(h string) bool {
	normalizedHostName := NormalizeHostname(h)
	return normalizedHostName != defaultHostname && !IsLocal(normalizedHostName)
}

// IsTenancy reports whether a non-normalized host name looks like a tenancy instance.
func IsTenancy(h string) bool {
	normalizedHostName := NormalizeHostname(h)
	return strings.HasSuffix(normalizedHostName, "."+tenancyHost)
}

// IsLocal reports whether a non-normalized host name looks like a local
// instance. It accepts an optional port number.
func IsLocal(h string) bool {
	switch parts := strings.Split(h, ":"); len(parts) {
	case 1, 2:
		h = parts[0]
	default:
		return false
	}

	return strings.EqualFold(h, localhost)
}

// TenantName extracts the tenant name from tenancy host name and
// reports whether it found the tenant name.
func TenantName(h string) (string, bool) {
	normalizedHostName := NormalizeHostname(h)
	return cutSuffix(normalizedHostName, "."+tenancyHost)
}

func isGarage(h string) bool {
	return strings.EqualFold(h, "garage.github.com")
}

// NormalizeHostname returns the canonical host name of a GitHub instance.
func NormalizeHostname(h string) string {
	hostname := strings.ToLower(h)
	if strings.HasSuffix(hostname, "."+defaultHostname) {
		return defaultHostname
	}
	if strings.HasSuffix(hostname, "."+localhost) {
		return localhost
	}
	if before, found := cutSuffix(hostname, "."+tenancyHost); found {
		idx := strings.LastIndex(before, ".")
		return fmt.Sprintf("%s.%s", before[idx+1:], tenancyHost)
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
	// Once we change Tenancy to no longer be treated as Enterprise, this
	// conditional can be removed as the flow will fall through to the bottom.
	// However, we can't do that until we've investigated all places in which
	// Tenancy is currently treated as Enterprise.
	if IsTenancy(hostname) {
		return fmt.Sprintf("https://api.%s/graphql", hostname)
	}
	if IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/api/graphql", hostname)
	}
	if IsLocal(hostname) {
		return fmt.Sprintf("http://api.%s/graphql", hostname)
	}
	return fmt.Sprintf("https://api.%s/graphql", hostname)
}

func RESTPrefix(hostname string) string {
	if isGarage(hostname) {
		return fmt.Sprintf("https://%s/api/v3/", hostname)
	}
	// Once we change Tenancy to no longer be treated as Enterprise, this
	// conditional can be removed as the flow will fall through to the bottom.
	// However, we can't do that until we've investigated all places in which
	// Tenancy is currently treated as Enterprise.
	if IsTenancy(hostname) {
		return fmt.Sprintf("https://api.%s/", hostname)
	}
	if IsEnterprise(hostname) {
		return fmt.Sprintf("https://%s/api/v3/", hostname)
	}
	if IsLocal(hostname) {
		return fmt.Sprintf("http://api.%s/", hostname)
	}
	return fmt.Sprintf("https://api.%s/", hostname)
}

func GistPrefix(hostname string) string {
	prefix := "https://"
	if IsLocal(hostname) {
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
	if IsLocal(hostname) {
		return fmt.Sprintf("%s/gist/", hostname)
	}
	return fmt.Sprintf("gist.%s/", hostname)
}

func HostPrefix(hostname string) string {
	if IsLocal(hostname) {
		return fmt.Sprintf("http://%s/", hostname)
	}
	return fmt.Sprintf("https://%s/", hostname)
}

// Backport strings.CutSuffix from Go 1.20.
func cutSuffix(s, suffix string) (string, bool) {
	if !strings.HasSuffix(s, suffix) {
		return s, false
	}
	return s[:len(s)-len(suffix)], true
}
