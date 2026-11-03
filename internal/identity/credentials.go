package identity

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cli/cli/v2/internal/gh"
)

// Credentials looks up a saved account without changing the globally active account.
type Credentials interface {
	TokenForUserWithRefresh(string, string) (gh.Credential, gh.RefreshStatus, error)
}

type accountConfig interface {
	HostForAPIHost(string) (string, bool)
	ResolveToken(host, owner, dir string) (gh.Credential, error)
}

// TokenResolver resolves effective credentials for the logical host named by api_host.
// HTTP header and redirect handling belong to the shared API authentication wrapper.
func TokenResolver(auth accountConfig) func(string) (string, error) {
	return func(host string) (string, error) {
		if canonical, ok := auth.HostForAPIHost(host); ok {
			host = canonical
		}
		credential, err := auth.ResolveToken(host, "", "")
		return credential.Token, err
	}
}

// CredentialFor resolves a bound credential, refusing environment overrides and missing accounts.
func CredentialFor(auth interface {
	Credentials
	SavedAccounts
}, id *Identity) (gh.Credential, error) {
	if err := CheckEnvironment(id.Host); err != nil {
		return gh.Credential{}, err
	}
	if warning := AccountWarning(auth, id); warning != "" {
		return gh.Credential{}, errors.New(warning)
	}
	credential, status, err := auth.TokenForUserWithRefresh(id.Host, id.Account)
	if err != nil {
		return gh.Credential{}, fmt.Errorf("identity %q: %w; sign in with 'gh auth login --hostname %s', or change the identity with 'gh auth identity edit %s --account <login>'", id.Name, err, id.Host, id.Name)
	}
	if status == gh.RefreshStatusExpired || credential.Token == "" {
		return gh.Credential{}, fmt.Errorf("identity %q has no usable credential for %s on %s; run 'gh auth login'", id.Name, id.Account, id.Host)
	}
	return credential, nil
}

// CheckEnvironment refuses token variables whose account cannot be matched to the identity.
func CheckEnvironment(host string) error {
	var names []string
	if host == "github.com" || strings.HasSuffix(host, ".ghe.com") {
		names = []string{"GH_TOKEN", "GITHUB_TOKEN"}
	} else {
		names = []string{"GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN"}
	}
	for _, name := range names {
		if os.Getenv(name) != "" {
			return fmt.Errorf("%s overrides saved accounts; unset it to use a bound identity", name)
		}
	}
	return nil
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
