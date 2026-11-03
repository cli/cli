package shared

import (
	"strings"

	"github.com/cli/cli/v2/internal/gh"
)

// AuthTokenRefreshable reports whether the token is stored by gh and can be
// renewed with `gh auth refresh`.
//
// TODO: this matches a token prefix itself. It could ask
// gh.AuthConfig.ActiveTokenType instead.
func AuthTokenRefreshable(token, src string) bool {
	return token != "" && !strings.HasSuffix(src, "_TOKEN") && strings.HasPrefix(token, "gho_")
}

func AuthTokenWriteable(authCfg gh.AuthConfig, hostname string) (string, bool) {
	cred := authCfg.ActiveToken(hostname)
	return cred.Source, (cred.Token == "" || !strings.HasSuffix(cred.Source, "_TOKEN"))
}
