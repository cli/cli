package shared

import (
	"strings"

	"github.com/cli/cli/v2/internal/gh"
)

// AuthTokenRefreshable reports whether the token is stored by gh and can be
// renewed with `gh auth refresh`.
func AuthTokenRefreshable(token, src string) bool {
	return token != "" && !strings.HasSuffix(src, "_TOKEN") && strings.HasPrefix(token, "gho_")
}

func AuthTokenWriteable(authCfg gh.AuthConfig, hostname string) (string, bool) {
	token, src := authCfg.ActiveToken(hostname)
	return src, (token == "" || !strings.HasSuffix(src, "_TOKEN"))
}
