//go:build integration

package integration

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"

	"github.com/eggsampler/acme/v3"
)

func init() {
	// Go tests get run in the directory their source code lives in. For these
	// test cases, that would be "test/integration." However, it's easier to
	// reference test data and config files for integration tests relative to the
	// root of the Boulder repo, so we run all of these tests from there instead.
	os.Chdir("../../")
}

var (
	OIDExtensionCTPoison = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 4, 3}
)

func random_domain() string {
	var bytes [3]byte
	rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:]) + ".com"
}

type client struct {
	acme.Account
	acme.Client
}

func makeClient(contacts ...string) (*client, error) {
	c, err := acme.NewClient("http://boulder.service.consul:4001/directory")
	if err != nil {
		return nil, fmt.Errorf("Error connecting to acme directory: %v", err)
	}
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("error creating private key: %v", err)
	}
	account, err := c.NewAccount(privKey, false, true, contacts...)
	if err != nil {
		return nil, err
	}
	return &client{account, c}, nil
}

func addHTTP01Response(token, keyAuthorization string) error {
	resp, err := http.Post("http://boulder.service.consul:8055/add-http01", "",
		bytes.NewBufferString(fmt.Sprintf(`{
		"token": "%s",
		"content": "%s"
	}`, token, keyAuthorization)))
	if err != nil {
		return fmt.Errorf("adding http-01 response: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("adding http-01 response: status %d", resp.StatusCode)
	}
	resp.Body.Close()
	return nil
}

func delHTTP01Response(token string) error {
	resp, err := http.Post("http://boulder.service.consul:8055/del-http01", "",
		bytes.NewBufferString(fmt.Sprintf(`{
		"token": "%s"
	}`, token)))
	if err != nil {
		return fmt.Errorf("deleting http-01 response: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("deleting http-01 response: status %d", resp.StatusCode)
	}
	return nil
}

func makeClientAndOrder(c *client, csrKey *ecdsa.PrivateKey, domains []string, cn bool, certToReplace *x509.Certificate) (*client, *acme.Order, error) {
	var err error
	if c == nil {
		c, err = makeClient()
		if err != nil {
			return nil, nil, err
		}
	}

	var ids []acme.Identifier
	for _, domain := range domains {
		ids = append(ids, acme.Identifier{Type: "dns", Value: domain})
	}
	var order acme.Order
	if certToReplace != nil {
		order, err = c.Client.ReplacementOrder(c.Account, certToReplace, ids)
	} else {
		order, err = c.Client.NewOrder(c.Account, ids)
	}
	if err != nil {
		return nil, nil, err
	}

	for _, authUrl := range order.Authorizations {
		auth, err := c.Client.FetchAuthorization(c.Account, authUrl)
		if err != nil {
			return nil, nil, fmt.Errorf("fetching authorization at %s: %s", authUrl, err)
		}

		chal, ok := auth.ChallengeMap[acme.ChallengeTypeHTTP01]
		if !ok {
			return nil, nil, fmt.Errorf("no HTTP challenge at %s", authUrl)
		}

		err = addHTTP01Response(chal.Token, chal.KeyAuthorization)
		if err != nil {
			return nil, nil, fmt.Errorf("adding HTTP-01 response: %s", err)
		}
		chal, err = c.Client.UpdateChallenge(c.Account, chal)
		if err != nil {
			delHTTP01Response(chal.Token)
			return nil, nil, fmt.Errorf("updating challenge: %s", err)
		}
		delHTTP01Response(chal.Token)
	}

	csr, err := makeCSR(csrKey, domains, cn)
	if err != nil {
		return nil, nil, err
	}

	order, err = c.Client.FinalizeOrder(c.Account, order, csr)
	if err != nil {
		return nil, nil, fmt.Errorf("finalizing order: %s", err)
	}

	return c, &order, nil
}

type issuanceResult struct {
	acme.Order
	certs []*x509.Certificate
}

func authAndIssue(c *client, csrKey *ecdsa.PrivateKey, domains []string, cn bool) (*issuanceResult, error) {
	var err error

	c, order, err := makeClientAndOrder(c, csrKey, domains, cn, nil)
	if err != nil {
		return nil, err
	}

	certs, err := c.Client.FetchCertificates(c.Account, order.Certificate)
	if err != nil {
		return nil, fmt.Errorf("fetching certificates: %s", err)
	}
	return &issuanceResult{*order, certs}, nil
}

type issuanceResultAllChains struct {
	acme.Order
	certs map[string][]*x509.Certificate
}

func authAndIssueFetchAllChains(c *client, csrKey *ecdsa.PrivateKey, domains []string, cn bool) (*issuanceResultAllChains, error) {
	c, order, err := makeClientAndOrder(c, csrKey, domains, cn, nil)
	if err != nil {
		return nil, err
	}

	// Retrieve all the certificate chains served by the WFE2.
	certs, err := c.Client.FetchAllCertificates(c.Account, order.Certificate)
	if err != nil {
		return nil, fmt.Errorf("fetching certificates: %s", err)
	}

	return &issuanceResultAllChains{*order, certs}, nil
}

func makeCSR(k *ecdsa.PrivateKey, domains []string, cn bool) (*x509.CertificateRequest, error) {
	var err error
	if k == nil {
		k, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generating certificate key: %s", err)
		}
	}

	tmpl := &x509.CertificateRequest{
		SignatureAlgorithm: x509.ECDSAWithSHA256,
		PublicKeyAlgorithm: x509.ECDSA,
		PublicKey:          k.Public(),
		DNSNames:           domains,
	}
	if cn {
		tmpl.Subject = pkix.Name{CommonName: domains[0]}
	}

	csrDer, err := x509.CreateCertificateRequest(rand.Reader, tmpl, k)
	if err != nil {
		return nil, fmt.Errorf("making csr: %s", err)
	}
	csr, err := x509.ParseCertificateRequest(csrDer)
	if err != nil {
		return nil, fmt.Errorf("parsing csr: %s", err)
	}
	return csr, nil
}
