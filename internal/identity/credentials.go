package identity

import (
	"fmt"

	"github.com/cli/cli/v2/internal/gh"
)

// Credentials looks up a saved account without changing the globally active account.
type Credentials interface {
	TokenForUserWithRefresh(string, string) (gh.Credential, gh.RefreshStatus, error)
}

type accountConfig interface {
	HostForAPIHost(string) (string, bool)
	ActiveTokenWithRefresh(string) (gh.Credential, gh.RefreshStatus, error)
}

// TokenResolver resolves effective credentials for the logical host named by api_host.
// HTTP header and redirect handling belong to the shared API authentication wrapper.
func TokenResolver(auth accountConfig) func(string) (string, error) {
	return func(host string) (string, error) {
		if canonical, ok := auth.HostForAPIHost(host); ok {
			host = canonical
		}
		credential, _, err := auth.ActiveTokenWithRefresh(host)
		if err != nil && credential.Token == "" {
			return "", err
		}
		return credential.Token, nil
	}
}

// CredentialForAccount resolves an explicitly requested saved account, never the active or environment token.
func CredentialForAccount(auth Credentials, host, account string) (gh.Credential, error) {
	credential, status, err := auth.TokenForUserWithRefresh(host, account)
	if err != nil {
		return gh.Credential{}, fmt.Errorf("account %q on %s: %w", account, host, err)
	}
	if status == gh.RefreshStatusExpired || credential.Token == "" {
		return gh.Credential{}, fmt.Errorf("account %q has no usable credential on %s; run 'gh auth login --hostname %s'", account, host, host)
	}
	return credential, nil
}
