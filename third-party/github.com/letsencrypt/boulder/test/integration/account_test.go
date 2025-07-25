//go:build integration

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/core"
)

// TestNewAccount tests that various new-account requests are handled correctly.
// It does not test malformed account contacts, as we no longer care about
// how well-formed the contact string is, since we no longer store them.
func TestNewAccount(t *testing.T) {
	t.Parallel()

	c, err := acme.NewClient("http://boulder.service.consul:4001/directory")
	if err != nil {
		t.Fatalf("failed to connect to acme directory: %s", err)
	}

	for _, tc := range []struct {
		name    string
		tos     bool
		contact []string
		wantErr string
	}{
		{
			name:    "No TOS agreement",
			tos:     false,
			contact: nil,
			wantErr: "must agree to terms of service",
		},
		{
			name:    "No contacts",
			tos:     true,
			contact: nil,
		},
		{
			name:    "One contact",
			tos:     true,
			contact: []string{"mailto:single@chisel.com"},
		},
		{
			name:    "Many contacts",
			tos:     true,
			contact: []string{"mailto:one@chisel.com", "mailto:two@chisel.com", "mailto:three@chisel.com"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatalf("failed to generate account key: %s", err)
			}

			acct, err := c.NewAccount(key, false, tc.tos, tc.contact...)

			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("NewAccount(tos: %t, contact: %#v) = %s, but want no err", tc.tos, tc.contact, err)
				}

				if len(acct.Contact) != 0 {
					t.Errorf("NewAccount(tos: %t, contact: %#v) = %#v, but want empty contacts", tc.tos, tc.contact, acct)
				}
			} else if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("NewAccount(tos: %t, contact: %#v) = %#v, but want error %q", tc.tos, tc.contact, acct, tc.wantErr)
				}

				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("NewAccount(tos: %t, contact: %#v) = %q, but want error %q", tc.tos, tc.contact, err, tc.wantErr)
				}
			}
		})
	}
}

func TestNewAccount_DuplicateKey(t *testing.T) {
	t.Parallel()

	c, err := acme.NewClient("http://boulder.service.consul:4001/directory")
	if err != nil {
		t.Fatalf("failed to connect to acme directory: %s", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate account key: %s", err)
	}

	// OnlyReturnExisting: true with a never-before-used key should result in an error.
	acct, err := c.NewAccount(key, true, true)
	if err == nil {
		t.Fatalf("NewAccount(key: 1, ore: true) = %#v, but want error notFound", acct)
	}

	// Create an account.
	acct, err = c.NewAccount(key, false, true)
	if err != nil {
		t.Fatalf("NewAccount(key: 1, ore: false) = %#v, but want success", err)
	}

	// A duplicate request should just return the same account.
	acct, err = c.NewAccount(key, false, true)
	if err != nil {
		t.Fatalf("NewAccount(key: 1, ore: false) = %#v, but want success", err)
	}

	// Specifying OnlyReturnExisting should do the same.
	acct, err = c.NewAccount(key, true, true)
	if err != nil {
		t.Fatalf("NewAccount(key: 1, ore: true) = %#v, but want success", err)
	}

	// Deactivate the account.
	acct, err = c.DeactivateAccount(acct)
	if err != nil {
		t.Fatalf("DeactivateAccount(acct: 1) = %#v, but want success", err)
	}

	// Now a new account request should return an error.
	acct, err = c.NewAccount(key, false, true)
	if err == nil {
		t.Fatalf("NewAccount(key: 1, ore: false) = %#v, but want error deactivated", acct)
	}

	// Specifying OnlyReturnExisting should do the same.
	acct, err = c.NewAccount(key, true, true)
	if err == nil {
		t.Fatalf("NewAccount(key: 1, ore: true) = %#v, but want error deactivated", acct)
	}
}

// TestAccountDeactivate tests that account deactivation works. It does not test
// that we reject requests for other account statuses, because eggsampler/acme
// wisely does not allow us to construct such malformed requests.
func TestAccountDeactivate(t *testing.T) {
	t.Parallel()

	c, err := acme.NewClient("http://boulder.service.consul:4001/directory")
	if err != nil {
		t.Fatalf("failed to connect to acme directory: %s", err)
	}

	acctKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate account key: %s", err)
	}

	account, err := c.NewAccount(acctKey, false, true, "mailto:hello@blackhole.net")
	if err != nil {
		t.Fatalf("failed to create initial account: %s", err)
	}

	got, err := c.DeactivateAccount(account)
	if err != nil {
		t.Errorf("unexpected error while deactivating account: %s", err)
	}

	if got.Status != string(core.StatusDeactivated) {
		t.Errorf("account deactivation should have set status to %q, instead got %q", core.StatusDeactivated, got.Status)
	}
}
