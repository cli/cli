//go:build integration

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"
	"github.com/miekg/dns"

	challtestsrvclient "github.com/letsencrypt/boulder/test/chall-test-srv-client"
	"github.com/letsencrypt/boulder/test/vars"
)

var expectedUserAgents = []string{"boulder", "remoteva-a", "remoteva-b", "remoteva-c"}

func collectUserAgentsFromDNSRequests(requests []challtestsrvclient.DNSRequest) []string {
	userAgents := make([]string, len(requests))
	for i, request := range requests {
		userAgents[i] = request.UserAgent
	}
	return userAgents
}

func assertUserAgentsLength(t *testing.T, got []string, checkType string) {
	t.Helper()

	if len(got) != 4 {
		t.Errorf("During %s, expected 4 User-Agents, got %d", checkType, len(got))
	}
}

func assertExpectedUserAgents(t *testing.T, got []string, checkType string) {
	t.Helper()

	for _, ua := range expectedUserAgents {
		if !slices.Contains(got, ua) {
			t.Errorf("During %s, expected User-Agent %q in %s (got %v)", checkType, ua, expectedUserAgents, got)
		}
	}
}

func TestMPICTLSALPN01(t *testing.T) {
	t.Parallel()

	client, err := makeClient()
	if err != nil {
		t.Fatalf("creating acme client: %s", err)
	}

	domain := randomDomain(t)

	order, err := client.Client.NewOrder(client.Account, []acme.Identifier{{Type: "dns", Value: domain}})
	if err != nil {
		t.Fatalf("creating order: %s", err)
	}

	authz, err := client.Client.FetchAuthorization(client.Account, order.Authorizations[0])
	if err != nil {
		t.Fatalf("fetching authorization: %s", err)
	}

	chal, ok := authz.ChallengeMap[acme.ChallengeTypeTLSALPN01]
	if !ok {
		t.Fatalf("no TLS-ALPN-01 challenge found in %#v", authz)
	}

	_, err = testSrvClient.AddARecord(domain, []string{"64.112.117.134"})
	if err != nil {
		t.Fatalf("adding A record: %s", err)
	}
	defer func() {
		testSrvClient.RemoveARecord(domain)
	}()

	_, err = testSrvClient.AddTLSALPN01Response(domain, chal.KeyAuthorization)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err = testSrvClient.RemoveTLSALPN01Response(domain)
		if err != nil {
			t.Fatal(err)
		}
	}()

	chal, err = client.Client.UpdateChallenge(client.Account, chal)
	if err != nil {
		t.Fatalf("completing TLS-ALPN-01 validation: %s", err)
	}

	validationEvents, err := testSrvClient.TLSALPN01RequestHistory(domain)
	if err != nil {
		t.Fatal(err)
	}
	if len(validationEvents) != 4 {
		t.Errorf("expected 4 validation events got %d", len(validationEvents))
	}

	dnsEvents, err := testSrvClient.DNSRequestHistory(domain)
	if err != nil {
		t.Fatal(err)
	}

	var caaEvents []challtestsrvclient.DNSRequest
	for _, event := range dnsEvents {
		if event.Question.Qtype == dns.TypeCAA {
			caaEvents = append(caaEvents, event)
		}
	}
	assertUserAgentsLength(t, collectUserAgentsFromDNSRequests(caaEvents), "CAA check")
	assertExpectedUserAgents(t, collectUserAgentsFromDNSRequests(caaEvents), "CAA check")
}

func TestMPICDNS01(t *testing.T) {
	t.Parallel()

	client, err := makeClient()
	if err != nil {
		t.Fatalf("creating acme client: %s", err)
	}

	domain := randomDomain(t)

	order, err := client.Client.NewOrder(client.Account, []acme.Identifier{{Type: "dns", Value: domain}})
	if err != nil {
		t.Fatalf("creating order: %s", err)
	}

	authz, err := client.Client.FetchAuthorization(client.Account, order.Authorizations[0])
	if err != nil {
		t.Fatalf("fetching authorization: %s", err)
	}

	chal, ok := authz.ChallengeMap[acme.ChallengeTypeDNS01]
	if !ok {
		t.Fatalf("no DNS challenge found in %#v", authz)
	}

	_, err = testSrvClient.AddDNS01Response(domain, chal.KeyAuthorization)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err = testSrvClient.RemoveDNS01Response(domain)
		if err != nil {
			t.Fatal(err)
		}
	}()

	chal, err = client.Client.UpdateChallenge(client.Account, chal)
	if err != nil {
		t.Fatalf("completing DNS-01 validation: %s", err)
	}

	challDomainDNSEvents, err := testSrvClient.DNSRequestHistory("_acme-challenge." + domain)
	if err != nil {
		t.Fatal(err)
	}

	var validationEvents []challtestsrvclient.DNSRequest
	for _, event := range challDomainDNSEvents {
		if event.Question.Qtype == dns.TypeTXT && event.Question.Name == "_acme-challenge."+domain+"." {
			validationEvents = append(validationEvents, event)
		}
	}
	assertUserAgentsLength(t, collectUserAgentsFromDNSRequests(validationEvents), "DNS-01 validation")
	assertExpectedUserAgents(t, collectUserAgentsFromDNSRequests(validationEvents), "DNS-01 validation")

	domainDNSEvents, err := testSrvClient.DNSRequestHistory(domain)
	if err != nil {
		t.Fatal(err)
	}

	var caaEvents []challtestsrvclient.DNSRequest
	for _, event := range domainDNSEvents {
		if event.Question.Qtype == dns.TypeCAA {
			caaEvents = append(caaEvents, event)
		}
	}
	assertUserAgentsLength(t, collectUserAgentsFromDNSRequests(caaEvents), "CAA check")
	assertExpectedUserAgents(t, collectUserAgentsFromDNSRequests(caaEvents), "CAA check")
}

func TestMPICHTTP01(t *testing.T) {
	t.Parallel()

	client, err := makeClient()
	if err != nil {
		t.Fatalf("creating acme client: %s", err)
	}

	domain := randomDomain(t)

	order, err := client.Client.NewOrder(client.Account, []acme.Identifier{{Type: "dns", Value: domain}})
	if err != nil {
		t.Fatalf("creating order: %s", err)
	}

	authz, err := client.Client.FetchAuthorization(client.Account, order.Authorizations[0])
	if err != nil {
		t.Fatalf("fetching authorization: %s", err)
	}

	chal, ok := authz.ChallengeMap[acme.ChallengeTypeHTTP01]
	if !ok {
		t.Fatalf("no HTTP challenge found in %#v", authz)
	}

	_, err = testSrvClient.AddHTTP01Response(chal.Token, chal.KeyAuthorization)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err = testSrvClient.RemoveHTTP01Response(chal.Token)
		if err != nil {
			t.Fatal(err)
		}
	}()

	chal, err = client.Client.UpdateChallenge(client.Account, chal)
	if err != nil {
		t.Fatalf("completing HTTP-01 validation: %s", err)
	}

	validationEvents, err := testSrvClient.HTTPRequestHistory(domain)
	if err != nil {
		t.Fatal(err)
	}

	var validationUAs []string
	for _, event := range validationEvents {
		if event.URL == "/.well-known/acme-challenge/"+chal.Token {
			validationUAs = append(validationUAs, event.UserAgent)
		}
	}
	assertUserAgentsLength(t, validationUAs, "HTTP-01 validation")
	assertExpectedUserAgents(t, validationUAs, "HTTP-01 validation")

	dnsEvents, err := testSrvClient.DNSRequestHistory(domain)
	if err != nil {
		t.Fatal(err)
	}

	var caaEvents []challtestsrvclient.DNSRequest
	for _, event := range dnsEvents {
		if event.Question.Qtype == dns.TypeCAA {
			caaEvents = append(caaEvents, event)
		}
	}

	assertUserAgentsLength(t, collectUserAgentsFromDNSRequests(caaEvents), "CAA check")
	assertExpectedUserAgents(t, collectUserAgentsFromDNSRequests(caaEvents), "CAA check")
}

func TestCAARechecking(t *testing.T) {
	t.Parallel()

	domain := randomDomain(t)
	idents := []acme.Identifier{{Type: "dns", Value: domain}}

	// Create an order and authorization, and fulfill the associated challenge.
	// This should put the authz into the "valid" state, since CAA checks passed.
	client, err := makeClient()
	if err != nil {
		t.Fatalf("creating acme client: %s", err)
	}

	order, err := client.Client.NewOrder(client.Account, idents)
	if err != nil {
		t.Fatalf("creating order: %s", err)
	}

	authz, err := client.Client.FetchAuthorization(client.Account, order.Authorizations[0])
	if err != nil {
		t.Fatalf("fetching authorization: %s", err)
	}

	chal, ok := authz.ChallengeMap[acme.ChallengeTypeHTTP01]
	if !ok {
		t.Fatalf("no HTTP challenge found in %#v", authz)
	}

	_, err = testSrvClient.AddHTTP01Response(chal.Token, chal.KeyAuthorization)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, err = testSrvClient.RemoveHTTP01Response(chal.Token)
		if err != nil {
			t.Fatal(err)
		}
	}()

	chal, err = client.Client.UpdateChallenge(client.Account, chal)
	if err != nil {
		t.Fatalf("completing HTTP-01 validation: %s", err)
	}

	// Manipulate the database so that it looks like the authz was validated
	// more than 8 hours ago.
	db, err := sql.Open("mysql", vars.DBConnSAIntegrationFullPerms)
	if err != nil {
		t.Fatalf("sql.Open: %s", err)
	}

	_, err = db.Exec(`UPDATE authz2 SET attemptedAt = ? WHERE identifierValue = ?`, time.Now().Add(-24*time.Hour).Format(time.DateTime), domain)
	if err != nil {
		t.Fatalf("updating authz attemptedAt timestamp: %s", err)
	}

	// Change the CAA record to now forbid issuance.
	_, err = testSrvClient.AddCAAIssue(domain, ";")
	if err != nil {
		t.Fatal(err)
	}

	// Try to finalize the order created above. Due to our db manipulation, this
	// should trigger a CAA recheck. And due to our challtestsrv manipulation,
	// that CAA recheck should fail. Therefore the whole finalize should fail.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating cert key: %s", err)
	}

	csr, err := makeCSR(key, idents, false)
	if err != nil {
		t.Fatalf("generating finalize csr: %s", err)
	}

	_, err = client.Client.FinalizeOrder(client.Account, order, csr)
	if err == nil {
		t.Errorf("expected finalize to fail, but got success")
	}
	if !strings.Contains(err.Error(), "CAA") {
		t.Errorf("expected finalize to fail due to CAA, but got: %s", err)
	}
}
