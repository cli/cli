//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"
	"github.com/jmhodges/clock"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/test/vars"
)

// crlUpdaterMu controls access to `runUpdater`, because two crl-updaters running
// at once will result in errors trying to lease shards that are already leased.
var crlUpdaterMu sync.Mutex

// runUpdater executes the crl-updater binary with the -runOnce flag, and
// returns when it completes.
func runUpdater(t *testing.T, configFile string) {
	t.Helper()
	crlUpdaterMu.Lock()
	defer crlUpdaterMu.Unlock()

	// Reset the s3-test-srv so that it only knows about serials contained in
	// this new batch of CRLs.
	resp, err := http.Post("http://localhost:4501/reset", "", bytes.NewReader([]byte{}))
	test.AssertNotError(t, err, "opening database connection")
	test.AssertEquals(t, resp.StatusCode, http.StatusOK)

	// Reset the "leasedUntil" column so this can be done alongside other
	// updater runs without worrying about unclean state.
	fc := clock.NewFake()
	db, err := sql.Open("mysql", vars.DBConnSAIntegrationFullPerms)
	test.AssertNotError(t, err, "opening database connection")
	_, err = db.Exec(`UPDATE crlShards SET leasedUntil = ?`, fc.Now().Add(-time.Minute))
	test.AssertNotError(t, err, "resetting leasedUntil column")

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

// TestCRLUpdaterStartup ensures that the crl-updater can start in daemon mode.
// We do this here instead of in startservers so that we can shut it down after
// we've confirmed it is running. It's important that it not be running while
// other CRL integration tests are running, because otherwise they fight over
// database leases, leading to flaky test failures.
func TestCRLUpdaterStartup(t *testing.T) {
	t.Parallel()

	crlUpdaterMu.Lock()
	defer crlUpdaterMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	binPath, err := filepath.Abs("bin/boulder")
	test.AssertNotError(t, err, "computing boulder binary path")

	configDir, ok := os.LookupEnv("BOULDER_CONFIG_DIR")
	test.Assert(t, ok, "failed to look up test config directory")
	configFile := path.Join(configDir, "crl-updater.json")

	c := exec.CommandContext(ctx, binPath, "crl-updater", "-config", configFile, "-debug-addr", ":8021")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		out, err := c.CombinedOutput()
		// Log the output and error, but only if the main goroutine couldn't connect
		// and declared the test failed.
		for _, line := range strings.Split(string(out), "\n") {
			t.Log(line)
		}
		t.Log(err)
		wg.Done()
	}()

	for attempt := range 10 {
		time.Sleep(core.RetryBackoff(attempt, 10*time.Millisecond, 1*time.Second, 2))

		conn, err := net.DialTimeout("tcp", "localhost:8021", 100*time.Millisecond)
		if errors.Is(err, syscall.ECONNREFUSED) {
			t.Logf("Connection attempt %d failed: %s", attempt, err)
			continue
		}
		if err != nil {
			t.Logf("Connection attempt %d failed unrecoverably: %s", attempt, err)
			t.Fail()
			break
		}
		t.Logf("Connection attempt %d succeeded", attempt)
		defer conn.Close()
		break
	}

	cancel()
	wg.Wait()
}

// TestCRLPipeline runs an end-to-end test of the crl issuance process, ensuring
// that the correct number of properly-formed and validly-signed CRLs are sent
// to our fake S3 service.
func TestCRLPipeline(t *testing.T) {
	// Basic setup.
	configDir, ok := os.LookupEnv("BOULDER_CONFIG_DIR")
	test.Assert(t, ok, "failed to look up test config directory")
	configFile := path.Join(configDir, "crl-updater.json")

	// Create a database connection so we can pretend to jump forward in time.
	db, err := sql.Open("mysql", vars.DBConnSAIntegrationFullPerms)
	test.AssertNotError(t, err, "creating database connection")

	// Issue a test certificate and save its serial number.
	client, err := makeClient()
	test.AssertNotError(t, err, "creating acme client")
	res, err := authAndIssue(client, nil, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "")
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

	// Confirm that the cert now *does* show up in the CRLs, with the right reason.
	runUpdater(t, configFile)
	resp, err = http.Get("http://localhost:4501/query?serial=" + serial)
	test.AssertNotError(t, err, "s3-test-srv GET /query failed")
	test.AssertEquals(t, resp.StatusCode, 200)
	reason, err := io.ReadAll(resp.Body)
	test.AssertNotError(t, err, "reading revocation reason")
	test.AssertEquals(t, string(reason), "5")
	resp.Body.Close()

	// Manipulate the database so it appears that the certificate is going to
	// expire very soon. The cert should still appear on the CRL.
	_, err = db.Exec("UPDATE revokedCertificates SET notAfterHour = ? WHERE serial = ?", time.Now().Add(time.Hour).Truncate(time.Hour).Format(time.DateTime), serial)
	test.AssertNotError(t, err, "updating expiry to near future")
	runUpdater(t, configFile)
	resp, err = http.Get("http://localhost:4501/query?serial=" + serial)
	test.AssertNotError(t, err, "s3-test-srv GET /query failed")
	test.AssertEquals(t, resp.StatusCode, 200)
	reason, err = io.ReadAll(resp.Body)
	test.AssertNotError(t, err, "reading revocation reason")
	test.AssertEquals(t, string(reason), "5")
	resp.Body.Close()

	// Again update the database so that the certificate has expired in the
	// very recent past. The cert should still appear on the CRL.
	_, err = db.Exec("UPDATE revokedCertificates SET notAfterHour = ? WHERE serial = ?", time.Now().Add(-time.Hour).Truncate(time.Hour).Format(time.DateTime), serial)
	test.AssertNotError(t, err, "updating expiry to recent past")
	runUpdater(t, configFile)
	resp, err = http.Get("http://localhost:4501/query?serial=" + serial)
	test.AssertNotError(t, err, "s3-test-srv GET /query failed")
	test.AssertEquals(t, resp.StatusCode, 200)
	reason, err = io.ReadAll(resp.Body)
	test.AssertNotError(t, err, "reading revocation reason")
	test.AssertEquals(t, string(reason), "5")
	resp.Body.Close()

	// Finally update the database so that the certificate expired several CRL
	// update cycles ago. The cert should now vanish from the CRL.
	_, err = db.Exec("UPDATE revokedCertificates SET notAfterHour = ? WHERE serial = ?", time.Now().Add(-48*time.Hour).Truncate(time.Hour).Format(time.DateTime), serial)
	test.AssertNotError(t, err, "updating expiry to far past")
	runUpdater(t, configFile)
	resp, err = http.Get("http://localhost:4501/query?serial=" + serial)
	test.AssertNotError(t, err, "s3-test-srv GET /query failed")
	test.AssertEquals(t, resp.StatusCode, 404)
	resp.Body.Close()
}

func TestCRLTemporalAndExplicitShardingCoexist(t *testing.T) {
	db, err := sql.Open("mysql", vars.DBConnSAIntegrationFullPerms)
	if err != nil {
		t.Fatalf("sql.Open: %s", err)
	}
	// Insert an old, revoked certificate in the certificateStatus table. Importantly this
	// serial has the 7f prefix, which is in test/config-next/crl-updater.json in the
	// `temporallyShardedPrefixes` list.
	// Random serial that is unique to this test.
	oldSerial := "7faa39be44fc95f3d19befe3cb715848e601"
	// This is hardcoded to match one of the issuer names in our integration test environment's
	// ca.json.
	issuerID := 43104258997432926
	_, err = db.Exec(`DELETE FROM certificateStatus WHERE serial = ?`, oldSerial)
	if err != nil {
		t.Fatalf("deleting old certificateStatus row: %s", err)
	}
	_, err = db.Exec(`
		INSERT INTO certificateStatus (serial, issuerID, notAfter, status, ocspLastUpdated, revokedDate, revokedReason, lastExpirationNagSent)
		VALUES (?, ?, ?, "revoked", NOW(), NOW(), 0, 0);`,
		oldSerial, issuerID, time.Now().Add(24*time.Hour).Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("inserting old certificateStatus row: %s", err)
	}

	client, err := makeClient()
	if err != nil {
		t.Fatalf("creating acme client: %s", err)
	}

	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("creating cert key: %s", err)
	}

	// Issue and revoke a certificate. In the config-next world, this will be an explicitly
	// sharded certificate. In the config world, this will be a temporally sharded certificate
	// (until we move `config` to explicit sharding). This means that in the config world,
	// this test only handles temporal sharding, but we don't config-gate it because it passes
	// in both worlds.
	result, err := authAndIssue(client, certKey, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "")
	if err != nil {
		t.Fatalf("authAndIssue: %s", err)
	}

	cert := result.certs[0]
	err = client.RevokeCertificate(
		client.Account,
		cert,
		client.PrivateKey,
		0,
	)
	if err != nil {
		t.Fatalf("revoking: %s", err)
	}

	runUpdater(t, path.Join(os.Getenv("BOULDER_CONFIG_DIR"), "crl-updater.json"))

	allCRLs := getAllCRLs(t)
	seen := make(map[string]bool)
	// Range over CRLs from all issuers, because the "old" certificate (7faa...) has a
	// different issuer than the "new" certificate issued by `authAndIssue`, which
	// has a random issuer.
	for _, crls := range allCRLs {
		for _, crl := range crls {
			for _, entry := range crl.RevokedCertificateEntries {
				serial := fmt.Sprintf("%x", entry.SerialNumber)
				if seen[serial] {
					t.Errorf("revoked certificate %s seen on multiple CRLs", serial)
				}
				seen[serial] = true
			}
		}
	}

	newSerial := fmt.Sprintf("%x", cert.SerialNumber)
	if !seen[newSerial] {
		t.Errorf("revoked certificate %s not seen on any CRL", newSerial)
	}
	if !seen[oldSerial] {
		t.Errorf("revoked certificate %s not seen on any CRL", oldSerial)
	}
}
