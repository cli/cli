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

// CheckAccounts augments an explanation with local account-registration problems.
// SSH keys and Git attribution do not depend on a saved API credential.
func (r *Report) CheckAccounts(accounts SavedAccounts) {
	if warning := AccountWarning(accounts, r.Selection.Identity); warning != "" {
		r.Warnings = append(r.Warnings, warning, "SSH can still work: logging out of gh does not remove SSH keys or change Git attribution.")
		r.Authentication.API.Status, r.Authentication.HTTPS.Status = "blocked", "blocked"
		detail := "This account is not signed in on " + r.Selection.Identity.Host + "."
		r.Authentication.API.Detail, r.Authentication.HTTPS.Detail = detail, detail
	}
}
