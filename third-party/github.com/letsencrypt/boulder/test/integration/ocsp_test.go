//go:build integration

package integration

import (
	"strings"
	"testing"

	"golang.org/x/crypto/ocsp"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/core"
	ocsp_helper "github.com/letsencrypt/boulder/test/ocsp/helper"
)

func TestOCSPHappyPath(t *testing.T) {
	t.Parallel()
	cert, err := authAndIssue(nil, nil, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "")
	if err != nil || len(cert.certs) < 1 {
		t.Fatal("failed to issue cert for OCSP testing")
	}
	resp, err := ocsp_helper.Req(cert.certs[0], ocspConf())
	if err != nil {
		t.Fatalf("want ocsp response, but got error: %s", err)
	}
	if resp.Status != ocsp.Good {
		t.Errorf("want ocsp status %#v, got %#v", ocsp.Good, resp.Status)
	}
}

func TestOCSPBadSerialPrefix(t *testing.T) {
	t.Parallel()
	res, err := authAndIssue(nil, nil, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "")
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
	_, err = ocsp_helper.Req(cert, ocspConf())
	if err == nil {
		t.Fatal("Expected error getting OCSP for request with invalid serial")
	}
}

func TestOCSPRejectedPrecertificate(t *testing.T) {
	t.Parallel()
	domain := random_domain()
	err := ctAddRejectHost(domain)
	if err != nil {
		t.Fatalf("adding ct-test-srv reject host: %s", err)
	}

	_, err = authAndIssue(nil, nil, []acme.Identifier{{Type: "dns", Value: domain}}, true, "")
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

	ocspConfig := ocspConf().WithExpectStatus(ocsp.Good)
	_, err = ocsp_helper.ReqDER(cert.Raw, ocspConfig)
	if err != nil {
		t.Errorf("requesting OCSP for rejected precertificate: %s", err)
	}
}
