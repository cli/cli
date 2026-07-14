package config

import (
	"github.com/cli/cli/v2/internal/gh"
)

// ResolveAPIHost returns the api_host override configured for hostname under the
// host in hosts.yml, or an empty string when none is set. It reads through the
// provided config so it honors the same source the rest of the command uses.
func ResolveAPIHost(cfg gh.Config, hostname string) string {
	if entry := cfg.GetOrDefault(hostname, "api_host"); entry.IsSome() {
		return entry.Unwrap().Value
	}
	return ""
}
