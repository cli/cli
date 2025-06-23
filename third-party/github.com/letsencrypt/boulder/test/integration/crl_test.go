//go:build integration

package integration

import (
	"database/sql"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/test/vars"
)

// runUpdater executes the crl-updater binary with the -runOnce flag, and
// returns when it completes.
func runUpdater(t *testing.T, configFile string) {
	t.Helper()

	binPath, err := filepath.Abs("bin/boulder")
	test.AssertNotError(t, err, "computing boulder binary path")

	c := exec.Command(binPath, "crl-updater", "-config", configFile, "-debug-addr", ":8022", "-runOnce")
	out, err := c.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		// Print the updater's stdout for debugging, but only if the test fails.
		t.Log(line)
	}
	test.AssertNotError(t, err, "crl-updater failed")
}

// TestCRLPipeline runs an end-to-end test of the crl issuance process, ensuring
// that the correct number of properly-formed and validly-signed CRLs are sent
// to our fake S3 service.
func TestCRLPipeline(t *testing.T) {
	// Basic setup.
	fc := clock.NewFake()
	configDir, ok := os.LookupEnv("BOULDER_CONFIG_DIR")
	test.Assert(t, ok, "failed to look up test config directory")
	configFile := path.Join(configDir, "crl-updater.json")

	// Reset the "leasedUntil" column so that this test isn't dependent on state
	// like priors runs of this test.
	db, err := sql.Open("mysql", vars.DBConnSAIntegrationFullPerms)
	test.AssertNotError(t, err, "opening database connection")
	_, err = db.Exec(`UPDATE crlShards SET leasedUntil = ?`, fc.Now().Add(-time.Minute))
	test.AssertNotError(t, err, "resetting leasedUntil column")

	// Issue a test certificate and save its serial number.
	client, err := makeClient()
	test.AssertNotError(t, err, "creating acme client")
	res, err := authAndIssue(client, nil, []string{random_domain()}, true)
	test.AssertNotError(t, err, "failed to create test certificate")
	cert := res.certs[0]
	serial := core.SerialToString(cert.SerialNumber)

	// Confirm that the cert does not yet show up as revoked in the CRLs.
	runUpdater(t, configFile)
	resp, err := http.Get("http://localhost:4501/query?serial=" + serial)
	test.AssertNotError(t, err, "s3-test-srv GET /query failed")
	test.AssertEquals(t, resp.StatusCode, 404)
	resp.Body.Close()

	// Revoke the certificate.
	err = client.RevokeCertificate(client.Account, cert, client.PrivateKey, 5)
	test.AssertNotError(t, err, "failed to revoke test certificate")

	// Reset the "leasedUntil" column to prepare for another round of CRLs.
	_, err = db.Exec(`UPDATE crlShards SET leasedUntil = ?`, fc.Now().Add(-time.Minute))
	test.AssertNotError(t, err, "resetting leasedUntil column")

	// Confirm that the cert now *does* show up in the CRLs.
	runUpdater(t, configFile)
	resp, err = http.Get("http://localhost:4501/query?serial=" + serial)
	test.AssertNotError(t, err, "s3-test-srv GET /query failed")
	test.AssertEquals(t, resp.StatusCode, 200)

	// Confirm that the revoked certificate entry has the correct reason.
	reason, err := io.ReadAll(resp.Body)
	test.AssertNotError(t, err, "reading revocation reason")
	test.AssertEquals(t, string(reason), "5")
	resp.Body.Close()
}
