package shared

import (
	"github.com/cli/cli/v2/internal/config"
)

const (
	oauthToken = "oauth_token"
)

func AuthTokenWriteable(authCfg *config.AuthConfig, hostname string) (string, bool) {
	token, src := authCfg.Token(hostname)
	return src, (token == "" || src == oauthToken)
}
