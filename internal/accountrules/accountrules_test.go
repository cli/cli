package accountrules

import (
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
)

func TestResolve(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows

	work := filepath.Join(home, "work")
	personal := filepath.Join(home, "personal")

	rules := gh.AccountRules{
		GitDir: map[string]string{
			"~/work/":     "octocat_acme@github.com",
			"~/personal/": "octocat",
		},
		Owner: map[string]string{
			"acme-corp": "octocat_acme@github.com",
			"octocat":   "octocat@github.com",
		},
	}

	tests := []struct {
		name        string
		rules       gh.AccountRules
		ctx         ResolveContext
		wantMatched bool
		wantUser    string
		wantHost    string
	}{
		{
			name:        "no rules, no override",
			rules:       gh.AccountRules{},
			ctx:         ResolveContext{Host: "github.com", Owner: "acme-corp", Cwd: work},
			wantMatched: false,
		},
		{
			name:        "override wins over everything",
			rules:       rules,
			ctx:         ResolveContext{Host: "github.com", Owner: "octocat", Cwd: personal, Override: "someoneelse@ghe.io"},
			wantMatched: true,
			wantUser:    "someoneelse",
			wantHost:    "ghe.io",
		},
		{
			name:        "override without host defaults to command host",
			rules:       rules,
			ctx:         ResolveContext{Host: "github.com", Override: "octocat_acme"},
			wantMatched: true,
			wantUser:    "octocat_acme",
			wantHost:    "github.com",
		},
		{
			name:        "owner rule matches (case-insensitive) and beats gitdir",
			rules:       rules,
			ctx:         ResolveContext{Host: "github.com", Owner: "ACME-Corp", Cwd: personal},
			wantMatched: true,
			wantUser:    "octocat_acme",
			wantHost:    "github.com",
		},
		{
			name:        "gitdir rule matches when no owner rule",
			rules:       rules,
			ctx:         ResolveContext{Host: "github.com", Owner: "unknown-org", Cwd: filepath.Join(work, "billing-api")},
			wantMatched: true,
			wantUser:    "octocat_acme",
			wantHost:    "github.com",
		},
		{
			name:        "gitdir rule without host defaults to command host",
			rules:       rules,
			ctx:         ResolveContext{Host: "github.com", Cwd: filepath.Join(personal, "dotfiles")},
			wantMatched: true,
			wantUser:    "octocat",
			wantHost:    "github.com",
		},
		{
			name:        "no match when owner and cwd are outside any rule",
			rules:       rules,
			ctx:         ResolveContext{Host: "github.com", Owner: "stranger", Cwd: filepath.Join(home, "elsewhere")},
			wantMatched: false,
		},
		{
			name:        "empty owner does not match owner rules",
			rules:       gh.AccountRules{Owner: map[string]string{"": "ghost"}},
			ctx:         ResolveContext{Host: "github.com", Owner: ""},
			wantMatched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acct, matched := Resolve(tt.rules, tt.ctx)
			if matched != tt.wantMatched {
				t.Fatalf("matched = %v, want %v", matched, tt.wantMatched)
			}
			if !matched {
				return
			}
			if acct.User != tt.wantUser {
				t.Errorf("user = %q, want %q", acct.User, tt.wantUser)
			}
			if acct.Host != tt.wantHost {
				t.Errorf("host = %q, want %q", acct.Host, tt.wantHost)
			}
		})
	}
}

func TestLongestPrefixWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	rules := gh.AccountRules{
		GitDir: map[string]string{
			"~/work/":             "broad@github.com",
			"~/work/acme-secure/": "narrow@github.com",
		},
	}

	ctx := ResolveContext{
		Host: "github.com",
		Cwd:  filepath.Join(home, "work", "acme-secure", "repo"),
	}

	acct, matched := Resolve(rules, ctx)
	if !matched {
		t.Fatal("expected a match")
	}
	if acct.User != "narrow" {
		t.Errorf("user = %q, want %q (longest prefix should win)", acct.User, "narrow")
	}
}

func TestPrefixDoesNotMatchSiblingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	rules := gh.AccountRules{
		GitDir: map[string]string{"~/work/": "octocat_acme@github.com"},
	}

	// ~/workshop must not match the ~/work/ rule.
	ctx := ResolveContext{Host: "github.com", Cwd: filepath.Join(home, "workshop", "repo")}

	if _, matched := Resolve(rules, ctx); matched {
		t.Error("expected no match for sibling directory ~/workshop")
	}
}
