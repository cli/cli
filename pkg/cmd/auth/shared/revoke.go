package shared

import (
	"net/http"
	"strings"

	"github.com/cli/cli/v2/internal/gh"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

// RevokeOAuthTokenIfChanged revokes a replaced GitHub CLI OAuth token.
func RevokeOAuthTokenIfChanged(
	httpClient *http.Client,
	hostname, previousToken, newToken string,
	revokeToken func(*http.Client, string, string) error,
) error {
	if previousToken == "" || previousToken == newToken {
		return nil
	}
	if !strings.HasPrefix(previousToken, string(gh.TokenTypeOAuth)) || ghauth.IsEnterprise(hostname) {
		return nil
	}

	return revokeToken(httpClient, hostname, previousToken)
}
