//go:build integration

package integration

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/test"
)

// TestFermat ensures that a certificate public key which can be factored using
// less than 100 rounds of Fermat's Algorithm is rejected.
func TestFermat(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name string
		p    string
		q    string
	}

	testCases := []testCase{
		{
			name: "canon printer (2048 bit, 1 round)",
			p:    "155536235030272749691472293262418471207550926406427515178205576891522284497518443889075039382254334975506248481615035474816604875321501901699955105345417152355947783063521554077194367454070647740704883461064399268622437721385112646454393005862535727615809073410746393326688230040267160616554768771412289114449",
			q:    "155536235030272749691472293262418471207550926406427515178205576891522284497518443889075039382254334975506248481615035474816604875321501901699955105345417152355947783063521554077194367454070647740704883461064399268622437721385112646454393005862535727615809073410746393326688230040267160616554768771412289114113",
		},
		{
			name: "innsbruck printer (4096 bit, 1 round)",
			p:    "25868808535211632564072019392873831934145242707953960515208595626279836366691068618582894100813803673421320899654654938470888358089618966238341690624345530870988951109006149164192566967552401505863871260691612081236189439839963332690997129144163260418447718577834226720411404568398865166471102885763673744513186211985402019037772108416694793355840983833695882936201196462579254234744648546792097397517107797153785052856301942321429858537224127598198913168345965493941246097657533085617002572245972336841716321849601971924830462771411171570422802773095537171762650402420866468579928479284978914972383512240254605625661",
			q:    "25868808535211632564072019392873831934145242707953960515208595626279836366691068618582894100813803673421320899654654938470888358089618966238341690624345530870988951109006149164192566967552401505863871260691612081236189439839963332690997129144163260418447718577834226720411404568398865166471102885763673744513186211985402019037772108416694793355840983833695882936201196462579254234744648546792097397517107797153785052856301942321429858537224127598198913168345965493941246097657533085617002572245972336841716321849601971924830462771411171570422802773095537171762650402420866468579928479284978914972383512240254605624819",
		},
		// Ideally we'd have a 2408-bit, nearly-100-rounds test case, but it turns
		// out purposefully generating keys that require 1 < N < 100 rounds to be
		// factored is surprisingly tricky.
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create a client and complete an HTTP-01 challenge for a fake domain.
			c, err := makeClient()
			test.AssertNotError(t, err, "creating acme client")

			domain := random_domain()

			order, err := c.Client.NewOrder(
				c.Account, []acme.Identifier{{Type: "dns", Value: domain}})
			test.AssertNotError(t, err, "creating new order")
			test.AssertEquals(t, len(order.Authorizations), 1)

			authUrl := order.Authorizations[0]

			auth, err := c.Client.FetchAuthorization(c.Account, authUrl)
			test.AssertNotError(t, err, "fetching authorization")

			chal, ok := auth.ChallengeMap[acme.ChallengeTypeHTTP01]
			test.Assert(t, ok, "getting HTTP-01 challenge")

			err = addHTTP01Response(chal.Token, chal.KeyAuthorization)
			defer delHTTP01Response(chal.Token)
			test.AssertNotError(t, err, "adding HTTP-01 response")

			chal, err = c.Client.UpdateChallenge(c.Account, chal)
			test.AssertNotError(t, err, "updating HTTP-01 challenge")

			// Reconstruct the public modulus N from the test case's prime factors.
			p, ok := new(big.Int).SetString(tc.p, 10)
			test.Assert(t, ok, "failed to create large prime")
			q, ok := new(big.Int).SetString(tc.q, 10)
			test.Assert(t, ok, "failed to create large prime")
			n := new(big.Int).Mul(p, q)

			// Reconstruct the private exponent D from the test case's prime factors.
			p_1 := new(big.Int).Sub(p, big.NewInt(1))
			q_1 := new(big.Int).Sub(q, big.NewInt(1))
			field := new(big.Int).Mul(p_1, q_1)
			d := new(big.Int).ModInverse(big.NewInt(65537), field)

			// Create a CSR containing the reconstructed pubkey and signed with the
			// reconstructed private key.
			pubkey := rsa.PublicKey{
				N: n,
				E: 65537,
			}

			privkey := rsa.PrivateKey{
				PublicKey: pubkey,
				D:         d,
				Primes:    []*big.Int{p, q},
			}

			csrDer, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
				SignatureAlgorithm: x509.SHA256WithRSA,
				PublicKeyAlgorithm: x509.RSA,
				PublicKey:          &pubkey,
				Subject:            pkix.Name{CommonName: domain},
				DNSNames:           []string{domain},
			}, &privkey)
			test.AssertNotError(t, err, "creating CSR")

			csr, err := x509.ParseCertificateRequest(csrDer)
			test.AssertNotError(t, err, "parsing CSR")

			// Finalizing the order should fail as we reject the public key.
			_, err = c.Client.FinalizeOrder(c.Account, order, csr)
			test.AssertError(t, err, "finalizing order")
			test.AssertContains(t, err.Error(), "urn:ietf:params:acme:error:badCSR")
			test.AssertContains(t, err.Error(), "key generated with factors too close together")
		})
	}
}
