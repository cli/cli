//go:build integration

package integration

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/core"
	ocsp_helper "github.com/letsencrypt/boulder/test/ocsp/helper"
)

// TODO(#5172): Fill out these test stubs.
func TestOCSPBadRequestMethod(t *testing.T) {
	return
}

func TestOCSPBadGetUrl(t *testing.T) {
	return
}

func TestOCSPBadGetBody(t *testing.T) {
	return
}

func TestOCSPBadPostBody(t *testing.T) {
	return
}

func TestOCSPBadHashAlgorithm(t *testing.T) {
	return
}

func TestOCSPBadIssuerCert(t *testing.T) {
	return
}

func TestOCSPBadSerialPrefix(t *testing.T) {
	t.Parallel()
	domain := random_domain()
	res, err := authAndIssue(nil, nil, []string{domain}, true)
	if err != nil || len(res.certs) < 1 {
		t.Fatal("Failed to issue dummy cert for OCSP testing")
	}
	cert := res.certs[0]
	// Increment the first byte of the cert's serial number by 1, making the
	// prefix invalid. This works because ocsp_helper.Req (and the underlying
	// ocsp.CreateRequest) completely ignore the cert's .Raw value.
	serialStr := []byte(core.SerialToString(cert.SerialNumber))
	serialStr[0] = serialStr[0] + 1
	cert.SerialNumber.SetString(string(serialStr), 16)
	_, err = ocsp_helper.Req(cert, ocsp_helper.DefaultConfig)
	if err == nil {
		t.Fatal("Expected error getting OCSP for request with invalid serial")
	}
}

func TestOCSPNonexistentSerial(t *testing.T) {
	return
}

func TestOCSPExpiredCert(t *testing.T) {
	return
}

func TestOCSPRejectedPrecertificate(t *testing.T) {
	t.Parallel()
	domain := random_domain()
	err := ctAddRejectHost(domain)
	if err != nil {
		t.Fatalf("adding ct-test-srv reject host: %s", err)
	}

	_, err = authAndIssue(nil, nil, []string{domain}, true)
	if err != nil {
		if !strings.Contains(err.Error(), "urn:ietf:params:acme:error:serverInternal") ||
			!strings.Contains(err.Error(), "SCT embedding") {
			t.Fatal(err)
		}
	}
	if err == nil {
		t.Fatal("expected error issuing for domain rejected by CT servers; got none")
	}

	// Try to find a precertificate matching the domain from one of the
	// configured ct-test-srv instances.
	cert, err := ctFindRejection([]string{domain})
	if err != nil || cert == nil {
		t.Fatalf("couldn't find rejected precert for %q", domain)
	}

	ocspConfig := ocsp_helper.DefaultConfig.WithExpectStatus(ocsp.Good)
	_, err = ocsp_helper.ReqDER(cert.Raw, ocspConfig)
	if err != nil {
		t.Errorf("requesting OCSP for rejected precertificate: %s", err)
	}
}
