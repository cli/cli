//go:build integration

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/test"
)

// randomDomain creates a random domain name for testing.
func randomDomain(t *testing.T) string {
	t.Helper()

	var bytes [4]byte
	_, err := rand.Read(bytes[:])
	if err != nil {
		test.AssertNotError(t, err, "Failed to generate random domain")
	}
	return fmt.Sprintf("%x.mail.com", bytes[:])
}

// getOAuthToken queries the pardot-test-srv for the current OAuth token.
func getOAuthToken(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile("test/secrets/salesforce_client_id")
	test.AssertNotError(t, err, "Failed to read Salesforce client ID")
	clientId := string(data)

	data, err = os.ReadFile("test/secrets/salesforce_client_secret")
	test.AssertNotError(t, err, "Failed to read Salesforce client secret")
	clientSecret := string(data)

	httpClient := http.DefaultClient
	resp, err := httpClient.PostForm("http://localhost:9601/services/oauth2/token", url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {strings.TrimSpace(clientId)},
		"client_secret": {strings.TrimSpace(clientSecret)},
	})
	test.AssertNotError(t, err, "Failed to fetch OAuth token")
	test.AssertEquals(t, resp.StatusCode, http.StatusOK)
	defer resp.Body.Close()

	var response struct {
		AccessToken string `json:"access_token"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&response)
	test.AssertNotError(t, err, "Failed to decode OAuth token")
	return response.AccessToken
}

// getCreatedContacts queries the pardot-test-srv for the list of created
// contacts.
func getCreatedContacts(t *testing.T, token string) []string {
	t.Helper()

	httpClient := http.DefaultClient
	req, err := http.NewRequest("GET", "http://localhost:9602/contacts", nil)
	test.AssertNotError(t, err, "Failed to create request")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	test.AssertNotError(t, err, "Failed to query contacts")
	test.AssertEquals(t, resp.StatusCode, http.StatusOK)
	defer resp.Body.Close()

	var got struct {
		Contacts []string `json:"contacts"`
	}
	decoder := json.NewDecoder(resp.Body)
	err = decoder.Decode(&got)
	test.AssertNotError(t, err, "Failed to decode contacts")
	return got.Contacts
}

// assertAllContactsReceived waits for the expected contacts to be received by
// pardot-test-srv. Retries every 50ms for up to 2 seconds and fails if the
// expected contacts are not received.
func assertAllContactsReceived(t *testing.T, token string, expect []string) {
	t.Helper()

	for attempt := range 20 {
		if attempt > 0 {
			time.Sleep(50 * time.Millisecond)
		}
		got := getCreatedContacts(t, token)

		allFound := true
		for _, e := range expect {
			if !slices.Contains(got, e) {
				allFound = false
				break
			}
		}
		if allFound {
			break
		}
		if attempt >= 19 {
			t.Fatalf("Expected contacts=%v to be received by pardot-test-srv, got contacts=%v", expect, got)
		}
	}
}

// TestContactsSentForNewAccount tests that contacts are dispatched to
// pardot-test-srv by the email-exporter when a new account is created.
func TestContactsSentForNewAccount(t *testing.T) {
	t.Parallel()

	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires WFE to be configured to use email-exporter")
	}

	token := getOAuthToken(t)
	domain := randomDomain(t)

	tests := []struct {
		name           string
		contacts       []string
		expectContacts []string
	}{
		{
			name:           "Single email",
			contacts:       []string{"mailto:example@" + domain},
			expectContacts: []string{"example@" + domain},
		},
		{
			name:           "Multiple emails",
			contacts:       []string{"mailto:example1@" + domain, "mailto:example2@" + domain},
			expectContacts: []string{"example1@" + domain, "example2@" + domain},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, err := acme.NewClient("http://boulder.service.consul:4001/directory")
			if err != nil {
				t.Fatalf("failed to connect to acme directory: %s", err)
			}

			acctKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatalf("failed to generate account key: %s", err)
			}

			_, err = c.NewAccount(acctKey, false, true, tt.contacts...)
			test.AssertNotError(t, err, "Failed to create initial account with contacts")
			assertAllContactsReceived(t, token, tt.expectContacts)
		})
	}
}
