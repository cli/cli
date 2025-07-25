//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/sa"
	"github.com/letsencrypt/boulder/test"
	ocsp_helper "github.com/letsencrypt/boulder/test/ocsp/helper"
	"github.com/letsencrypt/boulder/test/vars"
)

// getPrecertByName finds and parses a precertificate using the given hostname.
// It returns the most recent one.
func getPrecertByName(db *sql.DB, reversedName string) (*x509.Certificate, error) {
	reversedName = sa.EncodeIssuedName(reversedName)
	// Find the certificate from the precertificates table. We don't know the serial so
	// we have to look it up by name.
	var der []byte
	rows, err := db.Query(`
		SELECT der
		FROM issuedNames JOIN precertificates
		USING (serial)
		WHERE reversedName = ?
		ORDER BY issuedNames.id DESC
		LIMIT 1
	`, reversedName)
	for rows.Next() {
		err = rows.Scan(&der)
		if err != nil {
			return nil, err
		}
	}
	if der == nil {
		return nil, fmt.Errorf("no precertificate found for %q", reversedName)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

// expectOCSP500 queries OCSP for the given certificate and expects a 500 error.
func expectOCSP500(cert *x509.Certificate) error {
	_, err := ocsp_helper.Req(cert, ocspConf())
	if err == nil {
		return errors.New("Expected error getting OCSP for certificate that failed status storage")
	}

	var statusCodeError ocsp_helper.StatusCodeError
	if !errors.As(err, &statusCodeError) {
		return fmt.Errorf("Got wrong kind of error for OCSP. Expected status code error, got %s", err)
	} else if statusCodeError.Code != 500 {
		return fmt.Errorf("Got wrong error status for OCSP. Expected 500, got %d", statusCodeError.Code)
	}
	return nil
}

// TestIssuanceCertStorageFailed tests what happens when a storage RPC fails
// during issuance. Specifically, it tests that case where we successfully
// prepared and stored a linting certificate plus metadata, but after
// issuing the precertificate we failed to mark the certificate as "ready"
// to serve an OCSP "good" response.
//
// To do this, we need to mess with the database, because we want to cause
// a failure in one specific query, without control ever returning to the
// client. Fortunately we can do this with MySQL triggers.
//
// We also want to make sure we can revoke the precertificate, which we will
// assume exists (note that this different from the root program assumption
// that a final certificate exists for any precertificate, though it is
// similar in spirit).
func TestIssuanceCertStorageFailed(t *testing.T) {
	os.Setenv("DIRECTORY", "http://boulder.service.consul:4001/directory")

	ctx := context.Background()

	db, err := sql.Open("mysql", vars.DBConnSAIntegrationFullPerms)
	test.AssertNotError(t, err, "failed to open db connection")

	_, err = db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_ready`)
	test.AssertNotError(t, err, "failed to drop trigger")

	// Make a specific update to certificateStatus fail, for this test but not others.
	// To limit the effect to this one test, we make the trigger aware of a specific
	// hostname used in this test. Since the UPDATE to the certificateStatus table
	// doesn't include the hostname, we look it up in the issuedNames table, keyed
	// off of the serial being updated.
	// We limit this to UPDATEs that set the status to "good" because otherwise we
	// would fail to revoke the certificate later.
	// NOTE: CREATE and DROP TRIGGER do not work in prepared statements. Go's
	// database/sql will automatically try to use a prepared statement if you pass
	// any arguments to Exec besides the query itself, so don't do that.
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER fail_ready
		BEFORE UPDATE ON certificateStatus
		FOR EACH ROW BEGIN
		DECLARE reversedName1 VARCHAR(255);
		SELECT reversedName
		    INTO reversedName1
			FROM issuedNames
			WHERE serial = NEW.serial
			    AND reversedName LIKE "com.wantserror.%";
		IF NEW.status = "good" AND reversedName1 != "" THEN
			SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Pretend there was an error updating the certificateStatus';
		END IF;
		END
	`)
	test.AssertNotError(t, err, "failed to create trigger")

	defer db.ExecContext(ctx, `DROP TRIGGER IF EXISTS fail_ready`)

	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// ---- Test revocation by serial ----
	revokeMeDomain := "revokeme.wantserror.com"
	// This should fail because the trigger prevented setting the certificate status to "ready"
	_, err = authAndIssue(nil, certKey, []acme.Identifier{{Type: "dns", Value: revokeMeDomain}}, true, "")
	test.AssertError(t, err, "expected authAndIssue to fail")

	cert, err := getPrecertByName(db, revokeMeDomain)
	test.AssertNotError(t, err, "failed to get certificate by name")

	err = expectOCSP500(cert)
	test.AssertNotError(t, err, "expected 500 error from OCSP")

	// Revoke by invoking admin-revoker
	config := fmt.Sprintf("%s/%s", os.Getenv("BOULDER_CONFIG_DIR"), "admin.json")
	output, err := exec.Command(
		"./bin/admin",
		"-config", config,
		"-dry-run=false",
		"revoke-cert",
		"-serial", core.SerialToString(cert.SerialNumber),
		"-reason", "unspecified",
	).CombinedOutput()
	test.AssertNotError(t, err, fmt.Sprintf("revoking via admin-revoker: %s", string(output)))

	_, err = ocsp_helper.Req(cert,
		ocsp_helper.DefaultConfig.WithExpectStatus(ocsp.Revoked).WithExpectReason(ocsp.Unspecified))

	// ---- Test revocation by key ----
	blockMyKeyDomain := "blockmykey.wantserror.com"
	// This should fail because the trigger prevented setting the certificate status to "ready"
	_, err = authAndIssue(nil, certKey, []acme.Identifier{{Type: "dns", Value: blockMyKeyDomain}}, true, "")
	test.AssertError(t, err, "expected authAndIssue to fail")

	cert, err = getPrecertByName(db, blockMyKeyDomain)
	test.AssertNotError(t, err, "failed to get certificate by name")

	err = expectOCSP500(cert)
	test.AssertNotError(t, err, "expected 500 error from OCSP")

	// Time to revoke! We'll do it by creating a different, successful certificate
	// with the same key, then revoking that certificate for keyCompromise.
	revokeClient, err := makeClient()
	test.AssertNotError(t, err, "creating second acme client")
	res, err := authAndIssue(nil, certKey, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "")
	test.AssertNotError(t, err, "issuing second cert")

	successfulCert := res.certs[0]
	successfulCertIssuer := res.certs[1]
	err = revokeClient.RevokeCertificate(
		revokeClient.Account,
		successfulCert,
		certKey,
		1,
	)
	test.AssertNotError(t, err, "revoking second certificate")

	runUpdater(t, path.Join(os.Getenv("BOULDER_CONFIG_DIR"), "crl-updater.json"))
	fetchAndCheckRevoked(t, successfulCert, successfulCertIssuer, ocsp.KeyCompromise)

	for range 300 {
		_, err = ocsp_helper.Req(successfulCert,
			ocspConf().WithExpectStatus(ocsp.Revoked).WithExpectReason(ocsp.KeyCompromise))
		if err == nil {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	test.AssertNotError(t, err, "expected status to eventually become revoked")

	// Try to issue again with the same key, expecting an error because of the key is blocked.
	_, err = authAndIssue(nil, certKey, []acme.Identifier{{Type: "dns", Value: "123.example.com"}}, true, "")
	test.AssertError(t, err, "expected authAndIssue to fail")
	if !strings.Contains(err.Error(), "public key is forbidden") {
		t.Errorf("expected issuance to be rejected with a bad pubkey")
	}
}
