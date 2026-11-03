package identity

import (
	"fmt"
	"slices"
	"strings"
)

// SavedAccounts enumerates local account registrations without reading credentials.
type SavedAccounts interface {
	UsersForHost(string) []string
}

// AccountWarning explains a missing account without changing an identity or checking tokens.
func AccountWarning(accounts SavedAccounts, id *Identity) string {
	if id == nil || slices.ContainsFunc(accounts.UsersForHost(id.Host), func(user string) bool {
		return strings.EqualFold(user, id.Account)
	}) {
		return ""
	}
	return fmt.Sprintf("identity %q: account %q on %s is not signed in. Run 'gh auth login --hostname %s' to sign back in, 'gh auth identity edit %s --account <login>' to change accounts, or 'gh auth identity delete %s' to remove this identity and its bindings.",
		id.Name, id.Account, id.Host, id.Host, id.Name, id.Name)
}
