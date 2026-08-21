package factory

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/accountrules"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/utils"
)

// envTokenSources are the token sources that originate from environment
// variables. Tokens from these sources always take precedence and must never be
// overridden by account rules.
var envTokenSources = map[string]bool{
	"GH_TOKEN":                true,
	"GITHUB_TOKEN":            true,
	"GH_ENTERPRISE_TOKEN":     true,
	"GITHUB_ENTERPRISE_TOKEN": true,
}

func isEnvTokenSource(source string) bool {
	return envTokenSources[source]
}

// activeTokenGetter mirrors the (unexported) interface the API HTTP client
// consumes for token resolution. Both gh.AuthConfig and *accountResolver satisfy
// it, allowing the factory to swap in context-scoped resolution transparently.
type activeTokenGetter interface {
	ActiveToken(hostname string) (token string, source string)
}

// accountResolver is a tokenGetter that selects the acting account from local
// context (working directory and base repository owner) before falling back to
// the globally active account. It never mutates persistent state.
//
// Precedence for each ActiveToken call:
//  1. Environment token (GH_TOKEN, etc.) — always wins.
//  2. Explicit override (--account / GH_ACCOUNT).
//  3. Owner / gitdir rules.
//  4. Globally active account (identical to today's behavior).
type accountResolver struct {
	auth  gh.AuthConfig
	rules gh.AccountRules
	// ctxFn gathers the API-free resolution context. It is invoked at most once
	// per command invocation and memoized, since gathering it may run git.
	ctxFn func() accountrules.ResolveContext
	logf  func(format string, a ...any)

	once   sync.Once
	cached accountrules.ResolveContext
}

func (r *accountResolver) resolveContext() accountrules.ResolveContext {
	r.once.Do(func() { r.cached = r.ctxFn() })
	return r.cached
}

// ActiveToken implements the tokenGetter interface consumed by the HTTP client.
func (r *accountResolver) ActiveToken(hostname string) (string, string) {
	token, source := r.auth.ActiveToken(hostname)

	// Environment tokens always win; never override them.
	if isEnvTokenSource(source) {
		return token, source
	}

	ctx := r.resolveContext()
	ctx.Host = hostname

	acct, matched := accountrules.Resolve(r.rules, ctx)
	if !matched {
		return token, source
	}

	// Only apply the rule when it targets the host of this request.
	if acct.Host != "" && !strings.EqualFold(acct.Host, hostname) {
		return token, source
	}

	t, s, err := r.auth.TokenForUser(hostname, acct.User)
	if err != nil {
		r.logf("account: %s selected %s@%s but no stored token was found (%v); using active account\n", acct.Reason, acct.User, hostname, err)
		return token, source
	}

	r.logf("account: resolved %s@%s via %s\n", acct.User, hostname, acct.Reason)
	return t, s
}

// newAccountResolvingTokenGetter wraps auth with context-scoped account
// resolution. If no rules are configured and no override is set, it returns auth
// unchanged so behavior is byte-identical to today.
func newAccountResolvingTokenGetter(auth gh.AuthConfig, remotesFn func() (ghContext.Remotes, error), logOut io.Writer) activeTokenGetter {
	rules := auth.AccountRules()
	if rules.IsEmpty() && accountrules.Override() == "" {
		return auth
	}

	debug, _ := utils.IsDebugEnabled()
	logf := func(format string, a ...any) {
		if debug && logOut != nil {
			fmt.Fprintf(logOut, format, a...)
		}
	}

	return &accountResolver{
		auth:  auth,
		rules: rules,
		ctxFn: func() accountrules.ResolveContext {
			ctx := accountrules.ResolveContext{Override: accountrules.Override()}
			if cwd, err := os.Getwd(); err == nil {
				ctx.Cwd = cwd
			}
			// Owner is resolved API-free from the base git remote. Absence of a
			// repository (or remotes) simply leaves Owner empty.
			if remotesFn != nil {
				if remotes, err := remotesFn(); err == nil && len(remotes) > 0 {
					ctx.Owner = remotes[0].RepoOwner()
				}
			}
			return ctx
		},
		logf: logf,
	}
}
