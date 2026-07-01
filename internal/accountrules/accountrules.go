// Package accountrules resolves which authenticated account gh should act as for
// a given command, based on local, API-free context (the current working
// directory and the base repository owner) plus an optional explicit override.
//
// This makes multi-account usage automatic and context-scoped, similar to git's
// includeIf mechanism, instead of relying on the single global "active" account.
// Resolution never mutates any persistent state.
package accountrules

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/internal/gh"
)

// flagOverride holds the value of the global --account flag. It is set once
// during command execution and read lazily wherever account resolution happens.
var flagOverride string

// SetOverride records the value of the --account flag. An empty value is ignored
// in favor of the GH_ACCOUNT environment variable.
func SetOverride(v string) {
	flagOverride = v
}

// Override returns the explicit account override from the --account flag,
// falling back to the GH_ACCOUNT environment variable.
func Override() string {
	if flagOverride != "" {
		return flagOverride
	}
	return os.Getenv("GH_ACCOUNT")
}

// ResolveContext carries the local, API-free context used to select an account.
type ResolveContext struct {
	// Host is the host of the command being executed. It is used as the default
	// host for account values that do not specify one.
	Host string
	// Owner is the base repository owner (organization or user login), or "" if
	// it could not be determined without an API call (e.g. not in a repo).
	Owner string
	// Cwd is the current working directory, or "" if it could not be determined.
	Cwd string
	// Override is an explicit account selection from --account or GH_ACCOUNT.
	// When set it takes precedence over all configured rules.
	Override string
}

// Account is a resolved account selection.
type Account struct {
	User string
	Host string
	// Reason is a short, human-readable explanation of why this account was
	// selected, suitable for debug output and `gh auth status`.
	Reason string
}

// Resolve applies account selection precedence and returns the selected account
// along with matched=true. When nothing applies it returns matched=false, and
// the caller should fall back to the globally active account.
//
// Precedence (highest first):
//  1. Override (--account / GH_ACCOUNT)
//  2. Owner rule (exact, case-insensitive match on ctx.Owner)
//  3. GitDir rule (longest matching directory prefix of ctx.Cwd)
func Resolve(rules gh.AccountRules, ctx ResolveContext) (Account, bool) {
	if ctx.Override != "" {
		acct := parseAccount(ctx.Override, ctx.Host)
		acct.Reason = "explicit override"
		return acct, true
	}

	if ctx.Owner != "" {
		if v, ok := rules.Owner[strings.ToLower(ctx.Owner)]; ok {
			acct := parseAccount(v, ctx.Host)
			acct.Reason = "owner rule for " + ctx.Owner
			return acct, true
		}
	}

	if ctx.Cwd != "" && len(rules.GitDir) > 0 {
		if prefix, v, ok := longestPrefixMatch(rules.GitDir, ctx.Cwd); ok {
			acct := parseAccount(v, ctx.Host)
			acct.Reason = "gitdir rule for " + prefix
			return acct, true
		}
	}

	return Account{}, false
}

// parseAccount splits an account value of the form "user" or "user@host". When
// the host is omitted it defaults to defaultHost.
func parseAccount(value, defaultHost string) Account {
	value = strings.TrimSpace(value)
	if i := strings.LastIndex(value, "@"); i >= 0 {
		return Account{User: value[:i], Host: value[i+1:]}
	}
	return Account{User: value, Host: defaultHost}
}

// longestPrefixMatch returns the rule whose (tilde-expanded) directory prefix is
// the longest prefix of dir. Matching is done on cleaned, absolute paths so that
// "~/work" matches "/home/u/work/repo" but not "/home/u/workshop".
func longestPrefixMatch(rules map[string]string, dir string) (prefix, value string, ok bool) {
	cleanedDir := filepath.Clean(dir)

	var bestLen int
	for rawPrefix, v := range rules {
		p := filepath.Clean(expandTilde(rawPrefix))
		if !pathHasPrefix(cleanedDir, p) {
			continue
		}
		if len(p) > bestLen {
			bestLen = len(p)
			prefix = rawPrefix
			value = v
			ok = true
		}
	}
	return prefix, value, ok
}

// pathHasPrefix reports whether dir is equal to, or nested under, prefix.
func pathHasPrefix(dir, prefix string) bool {
	if dir == prefix {
		return true
	}
	if prefix == string(filepath.Separator) {
		return strings.HasPrefix(dir, prefix)
	}
	return strings.HasPrefix(dir, prefix+string(filepath.Separator))
}

// expandTilde expands a leading "~" to the user's home directory. Directory
// prefixes in config are conventionally written with a forward slash (e.g.
// "~/work/") regardless of platform, so both "/" and the OS path separator are
// accepted after the tilde.
func expandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(p, "~")))
		}
	}
	return p
}
