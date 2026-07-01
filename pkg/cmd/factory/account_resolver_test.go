package factory

import (
	"testing"

	"github.com/cli/cli/v2/internal/accountrules"
	"github.com/cli/cli/v2/internal/gh"
)

// fakeAuth implements gh.AuthConfig by embedding the interface (nil) and
// overriding only the methods the account resolver exercises. Any other method
// call would panic, which is what we want in these focused tests.
type fakeAuth struct {
	gh.AuthConfig
	activeToken     func(string) (string, string)
	tokenForUser    func(string, string) (string, string, error)
	tokenForUserHit *bool
}

func (f fakeAuth) ActiveToken(host string) (string, string) { return f.activeToken(host) }

func (f fakeAuth) TokenForUser(host, user string) (string, string, error) {
	if f.tokenForUserHit != nil {
		*f.tokenForUserHit = true
	}
	return f.tokenForUser(host, user)
}

func newResolver(auth gh.AuthConfig, rules gh.AccountRules, ctx accountrules.ResolveContext) *accountResolver {
	return &accountResolver{
		auth:  auth,
		rules: rules,
		ctxFn: func() accountrules.ResolveContext { return ctx },
		logf:  func(string, ...any) {},
	}
}

func TestAccountResolver_EnvTokenAlwaysWins(t *testing.T) {
	hit := false
	auth := fakeAuth{
		activeToken:     func(string) (string, string) { return "env-token", "GH_TOKEN" },
		tokenForUser:    func(string, string) (string, string, error) { return "should-not-be-used", "keyring", nil },
		tokenForUserHit: &hit,
	}
	rules := gh.AccountRules{Owner: map[string]string{"acme-corp": "octocat_acme@github.com"}}

	r := newResolver(auth, rules, accountrules.ResolveContext{Owner: "acme-corp"})

	token, source := r.ActiveToken("github.com")
	if token != "env-token" || source != "GH_TOKEN" {
		t.Fatalf("got (%q, %q), want env token to win", token, source)
	}
	if hit {
		t.Error("TokenForUser must not be called when an env token is present")
	}
}

func TestAccountResolver_OwnerRuleApplies(t *testing.T) {
	auth := fakeAuth{
		activeToken: func(string) (string, string) { return "active-token", "oauth_token" },
		tokenForUser: func(host, user string) (string, string, error) {
			if user != "octocat_acme" {
				t.Errorf("TokenForUser called with user %q, want octocat_acme", user)
			}
			return "acme-token", "keyring", nil
		},
	}
	rules := gh.AccountRules{Owner: map[string]string{"acme-corp": "octocat_acme@github.com"}}

	r := newResolver(auth, rules, accountrules.ResolveContext{Owner: "acme-corp"})

	token, source := r.ActiveToken("github.com")
	if token != "acme-token" || source != "keyring" {
		t.Fatalf("got (%q, %q), want the resolved account token", token, source)
	}
}

func TestAccountResolver_NoMatchFallsBackToActive(t *testing.T) {
	auth := fakeAuth{
		activeToken:  func(string) (string, string) { return "active-token", "oauth_token" },
		tokenForUser: func(string, string) (string, string, error) { return "", "", nil },
	}
	rules := gh.AccountRules{Owner: map[string]string{"acme-corp": "octocat_acme@github.com"}}

	r := newResolver(auth, rules, accountrules.ResolveContext{Owner: "some-other-org"})

	token, source := r.ActiveToken("github.com")
	if token != "active-token" || source != "oauth_token" {
		t.Fatalf("got (%q, %q), want fallback to active account", token, source)
	}
}

func TestAccountResolver_MissingTokenForUserFallsBack(t *testing.T) {
	auth := fakeAuth{
		activeToken:  func(string) (string, string) { return "active-token", "oauth_token" },
		tokenForUser: func(string, string) (string, string, error) { return "", "default", errNoToken{} },
	}
	rules := gh.AccountRules{Owner: map[string]string{"acme-corp": "octocat_acme@github.com"}}

	r := newResolver(auth, rules, accountrules.ResolveContext{Owner: "acme-corp"})

	token, source := r.ActiveToken("github.com")
	if token != "active-token" || source != "oauth_token" {
		t.Fatalf("got (%q, %q), want fallback when no stored token exists", token, source)
	}
}

func TestAccountResolver_HostMismatchIsSkipped(t *testing.T) {
	hit := false
	auth := fakeAuth{
		activeToken:     func(string) (string, string) { return "active-token", "oauth_token" },
		tokenForUser:    func(string, string) (string, string, error) { return "other", "keyring", nil },
		tokenForUserHit: &hit,
	}
	// Rule targets ghe.io, but the request is for github.com.
	rules := gh.AccountRules{Owner: map[string]string{"acme-corp": "octocat_acme@ghe.io"}}

	r := newResolver(auth, rules, accountrules.ResolveContext{Owner: "acme-corp"})

	token, _ := r.ActiveToken("github.com")
	if token != "active-token" {
		t.Fatalf("got %q, want active token when rule host differs from request host", token)
	}
	if hit {
		t.Error("TokenForUser must not be called when the rule targets a different host")
	}
}

func TestNewAccountResolvingTokenGetter_PassthroughWhenUnconfigured(t *testing.T) {
	t.Setenv("GH_ACCOUNT", "")
	auth := fakeAuth{
		AuthConfig:  stubAuthConfigWithEmptyRules{},
		activeToken: func(string) (string, string) { return "active-token", "oauth_token" },
	}

	got := newAccountResolvingTokenGetter(auth, nil, nil)

	// With no rules and no override, the original auth config is returned as-is.
	if _, ok := got.(*accountResolver); ok {
		t.Fatal("expected passthrough to the original auth config, got a resolver wrapper")
	}
}

// stubAuthConfigWithEmptyRules provides AccountRules() returning empty rules for
// the passthrough test, since fakeAuth embeds a nil AuthConfig.
type stubAuthConfigWithEmptyRules struct{ gh.AuthConfig }

func (stubAuthConfigWithEmptyRules) AccountRules() gh.AccountRules { return gh.AccountRules{} }

type errNoToken struct{}

func (errNoToken) Error() string { return "no token found" }
