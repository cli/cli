package sa

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"math/bits"
	mrand "math/rand"
	"net"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-sql-driver/mysql"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/db"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/test/vars"
)

var log = blog.UseMock()
var ctx = context.Background()

var (
	theKey = `{
    "kty": "RSA",
    "n": "n4EPtAOCc9AlkeQHPzHStgAbgs7bTZLwUBZdR8_KuKPEHLd4rHVTeT-O-XV2jRojdNhxJWTDvNd7nqQ0VEiZQHz_AJmSCpMaJMRBSFKrKb2wqVwGU_NsYOYL-QtiWN2lbzcEe6XC0dApr5ydQLrHqkHHig3RBordaZ6Aj-oBHqFEHYpPe7Tpe-OfVfHd1E6cS6M1FZcD1NNLYD5lFHpPI9bTwJlsde3uhGqC0ZCuEHg8lhzwOHrtIQbS0FVbb9k3-tVTU4fg_3L_vniUFAKwuCLqKnS2BYwdq_mzSnbLY7h_qixoR7jig3__kRhuaxwUkRz5iaiQkqgc5gHdrNP5zw",
    "e": "AQAB"
}`
)

type fakeServerStream[T any] struct {
	grpc.ServerStream
	output chan<- *T
}

func (s *fakeServerStream[T]) Send(msg *T) error {
	s.output <- msg
	return nil
}

func (s *fakeServerStream[T]) Context() context.Context {
	return context.Background()
}

// initSA constructs a SQLStorageAuthority and a clean up function that should
// be defer'ed to the end of the test.
func initSA(t *testing.T) (*SQLStorageAuthority, clock.FakeClock, func()) {
	t.Helper()
	features.Reset()

	dbMap, err := DBMapForTest(vars.DBConnSA)
	if err != nil {
		t.Fatalf("Failed to create dbMap: %s", err)
	}

	dbIncidentsMap, err := DBMapForTest(vars.DBConnIncidents)
	if err != nil {
		t.Fatalf("Failed to create dbMap: %s", err)
	}

	fc := clock.NewFake()
	fc.Set(time.Date(2015, 3, 4, 5, 0, 0, 0, time.UTC))

	saro, err := NewSQLStorageAuthorityRO(dbMap, dbIncidentsMap, metrics.NoopRegisterer, 1, 0, fc, log)
	if err != nil {
		t.Fatalf("Failed to create SA: %s", err)
	}

	sa, err := NewSQLStorageAuthorityWrapping(saro, dbMap, metrics.NoopRegisterer)
	if err != nil {
		t.Fatalf("Failed to create SA: %s", err)
	}

	return sa, fc, test.ResetBoulderTestDatabase(t)
}

// CreateWorkingTestRegistration inserts a new, correct Registration into the
// given SA.
func createWorkingRegistration(t *testing.T, sa *SQLStorageAuthority) *corepb.Registration {
	initialIP, _ := net.ParseIP("88.77.66.11").MarshalText()
	reg, err := sa.NewRegistration(context.Background(), &corepb.Registration{
		Key:       []byte(theKey),
		Contact:   []string{"mailto:foo@example.com"},
		InitialIP: initialIP,
		CreatedAt: timestamppb.New(time.Date(2003, 5, 10, 0, 0, 0, 0, time.UTC)),
		Status:    string(core.StatusValid),
	})
	if err != nil {
		t.Fatalf("Unable to create new registration: %s", err)
	}
	return reg
}

func createPendingAuthorization(t *testing.T, sa *SQLStorageAuthority, domain string, exp time.Time) int64 {
	t.Helper()

	tokenStr := core.NewToken()
	token, err := base64.RawURLEncoding.DecodeString(tokenStr)
	test.AssertNotError(t, err, "computing test authorization challenge token")

	am := authzModel{
		IdentifierType:  0, // dnsName
		IdentifierValue: domain,
		RegistrationID:  1,
		Status:          statusToUint[core.StatusPending],
		Expires:         exp,
		Challenges:      1 << challTypeToUint[string(core.ChallengeTypeHTTP01)],
		Token:           token,
	}

	err = sa.dbMap.Insert(context.Background(), &am)
	test.AssertNotError(t, err, "creating test authorization")

	return am.ID
}

func createFinalizedAuthorization(t *testing.T, sa *SQLStorageAuthority, domain string, exp time.Time,
	status string, attemptedAt time.Time) int64 {
	t.Helper()
	pendingID := createPendingAuthorization(t, sa, domain, exp)
	attempted := string(core.ChallengeTypeHTTP01)
	_, err := sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id:          pendingID,
		Status:      status,
		Expires:     timestamppb.New(exp),
		Attempted:   attempted,
		AttemptedAt: timestamppb.New(attemptedAt),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorizations2 failed")
	return pendingID
}

func goodTestJWK() *jose.JSONWebKey {
	var jwk jose.JSONWebKey
	err := json.Unmarshal([]byte(theKey), &jwk)
	if err != nil {
		panic("known-good theKey is no longer known-good")
	}
	return &jwk
}

func TestAddRegistration(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	jwk := goodTestJWK()
	jwkJSON, _ := jwk.MarshalJSON()

	contacts := []string{"mailto:foo@example.com"}
	initialIP, _ := net.ParseIP("43.34.43.34").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       jwkJSON,
		Contact:   contacts,
		InitialIP: initialIP,
	})
	if err != nil {
		t.Fatalf("Couldn't create new registration: %s", err)
	}
	test.Assert(t, reg.Id != 0, "ID shouldn't be 0")
	test.AssertDeepEquals(t, reg.Contact, contacts)

	_, err = sa.GetRegistration(ctx, &sapb.RegistrationID{Id: 0})
	test.AssertError(t, err, "Registration object for ID 0 was returned")

	dbReg, err := sa.GetRegistration(ctx, &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, fmt.Sprintf("Couldn't get registration with ID %v", reg.Id))

	createdAt := clk.Now()
	test.AssertEquals(t, dbReg.Id, reg.Id)
	test.AssertByteEquals(t, dbReg.Key, jwkJSON)
	test.AssertDeepEquals(t, dbReg.CreatedAt.AsTime(), createdAt)

	initialIP, _ = net.ParseIP("72.72.72.72").MarshalText()
	newReg := &corepb.Registration{
		Id:        reg.Id,
		Key:       jwkJSON,
		Contact:   []string{"test.com"},
		InitialIP: initialIP,
		Agreement: "yes",
	}
	_, err = sa.UpdateRegistration(ctx, newReg)
	test.AssertNotError(t, err, fmt.Sprintf("Couldn't get registration with ID %v", reg.Id))
	dbReg, err = sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: jwkJSON})
	test.AssertNotError(t, err, "Couldn't get registration by key")

	test.AssertEquals(t, dbReg.Id, newReg.Id)
	test.AssertEquals(t, dbReg.Agreement, newReg.Agreement)

	anotherKey := `{
		"kty":"RSA",
		"n": "vd7rZIoTLEe-z1_8G1FcXSw9CQFEJgV4g9V277sER7yx5Qjz_Pkf2YVth6wwwFJEmzc0hoKY-MMYFNwBE4hQHw",
		"e":"AQAB"
	}`

	_, err = sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: []byte(anotherKey)})
	test.AssertError(t, err, "Registration object for invalid key was returned")
}

func TestNoSuchRegistrationErrors(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	_, err := sa.GetRegistration(ctx, &sapb.RegistrationID{Id: 100})
	test.AssertErrorIs(t, err, berrors.NotFound)

	jwk := goodTestJWK()
	jwkJSON, _ := jwk.MarshalJSON()

	_, err = sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: jwkJSON})
	test.AssertErrorIs(t, err, berrors.NotFound)

	_, err = sa.UpdateRegistration(ctx, &corepb.Registration{Id: 100, Key: jwkJSON, InitialIP: []byte("foo")})
	test.AssertErrorIs(t, err, berrors.NotFound)
}

func TestSelectRegistration(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()
	var ctx = context.Background()
	jwk := goodTestJWK()
	jwkJSON, _ := jwk.MarshalJSON()
	sha, err := core.KeyDigestB64(jwk.Key)
	test.AssertNotError(t, err, "couldn't parse jwk.Key")

	initialIP, _ := net.ParseIP("43.34.43.34").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       jwkJSON,
		Contact:   []string{"mailto:foo@example.com"},
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, fmt.Sprintf("couldn't create new registration: %s", err))
	test.Assert(t, reg.Id != 0, "ID shouldn't be 0")

	_, err = selectRegistration(ctx, sa.dbMap, "id", reg.Id)
	test.AssertNotError(t, err, "selecting by id should work")
	_, err = selectRegistration(ctx, sa.dbMap, "jwk_sha256", sha)
	test.AssertNotError(t, err, "selecting by jwk_sha256 should work")
	_, err = selectRegistration(ctx, sa.dbMap, "initialIP", reg.Id)
	test.AssertError(t, err, "selecting by any other column should not work")
}

func TestReplicationLagRetries(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	// First, set the lagFactor to 0. Neither selecting a real registration nor
	// selecting a nonexistent registration should cause the clock to advance.
	sa.lagFactor = 0
	start := clk.Now()

	_, err := sa.GetRegistration(ctx, &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, "selecting extant registration")
	test.AssertEquals(t, clk.Now(), start)
	test.AssertMetricWithLabelsEquals(t, sa.lagFactorCounter, prometheus.Labels{"method": "GetRegistration", "result": "notfound"}, 0)

	_, err = sa.GetRegistration(ctx, &sapb.RegistrationID{Id: reg.Id + 1})
	test.AssertError(t, err, "selecting nonexistent registration")
	test.AssertEquals(t, clk.Now(), start)
	// With lagFactor disabled, we should never enter the retry codepath, as a
	// result the metric should not increment.
	test.AssertMetricWithLabelsEquals(t, sa.lagFactorCounter, prometheus.Labels{"method": "GetRegistration", "result": "notfound"}, 0)

	// Now, set the lagFactor to 1. Trying to select a nonexistent registration
	// should cause the clock to advance when GetRegistration sleeps and retries.
	sa.lagFactor = 1
	start = clk.Now()

	_, err = sa.GetRegistration(ctx, &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, "selecting extant registration")
	test.AssertEquals(t, clk.Now(), start)
	// lagFactor is enabled, but the registration exists.
	test.AssertMetricWithLabelsEquals(t, sa.lagFactorCounter, prometheus.Labels{"method": "GetRegistration", "result": "notfound"}, 0)

	_, err = sa.GetRegistration(ctx, &sapb.RegistrationID{Id: reg.Id + 1})
	test.AssertError(t, err, "selecting nonexistent registration")
	test.AssertEquals(t, clk.Now(), start.Add(1))
	// With lagFactor enabled, we should enter the retry codepath and as a result
	// the metric should increment.
	test.AssertMetricWithLabelsEquals(t, sa.lagFactorCounter, prometheus.Labels{"method": "GetRegistration", "result": "notfound"}, 1)
}

// findIssuedName is a small helper test function to directly query the
// issuedNames table for a given name to find a serial (or return an err).
func findIssuedName(ctx context.Context, dbMap db.OneSelector, name string) (string, error) {
	var issuedNamesSerial string
	err := dbMap.SelectOne(
		ctx,
		&issuedNamesSerial,
		`SELECT serial FROM issuedNames
		WHERE reversedName = ?
		ORDER BY notBefore DESC
		LIMIT 1`,
		ReverseName(name))
	return issuedNamesSerial, err
}

func TestAddSerial(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)
	serial, testCert := test.ThrowAwayCert(t, clk)

	_, err := sa.AddSerial(context.Background(), &sapb.AddSerialRequest{
		RegID:   reg.Id,
		Created: timestamppb.New(testCert.NotBefore),
		Expires: timestamppb.New(testCert.NotAfter),
	})
	test.AssertError(t, err, "adding without serial should fail")

	_, err = sa.AddSerial(context.Background(), &sapb.AddSerialRequest{
		Serial:  serial,
		Created: timestamppb.New(testCert.NotBefore),
		Expires: timestamppb.New(testCert.NotAfter),
	})
	test.AssertError(t, err, "adding without regid should fail")

	_, err = sa.AddSerial(context.Background(), &sapb.AddSerialRequest{
		Serial:  serial,
		RegID:   reg.Id,
		Expires: timestamppb.New(testCert.NotAfter),
	})
	test.AssertError(t, err, "adding without created should fail")

	_, err = sa.AddSerial(context.Background(), &sapb.AddSerialRequest{
		Serial:  serial,
		RegID:   reg.Id,
		Created: timestamppb.New(testCert.NotBefore),
	})
	test.AssertError(t, err, "adding without expires should fail")

	_, err = sa.AddSerial(context.Background(), &sapb.AddSerialRequest{
		Serial:  serial,
		RegID:   reg.Id,
		Created: timestamppb.New(testCert.NotBefore),
		Expires: timestamppb.New(testCert.NotAfter),
	})
	test.AssertNotError(t, err, "adding serial should have succeeded")
}

func TestGetSerialMetadata(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)
	serial, _ := test.ThrowAwayCert(t, clk)

	_, err := sa.GetSerialMetadata(context.Background(), &sapb.Serial{Serial: serial})
	test.AssertError(t, err, "getting nonexistent serial should have failed")

	now := clk.Now()
	hourLater := now.Add(time.Hour)
	_, err = sa.AddSerial(context.Background(), &sapb.AddSerialRequest{
		Serial:  serial,
		RegID:   reg.Id,
		Created: timestamppb.New(now),
		Expires: timestamppb.New(hourLater),
	})
	test.AssertNotError(t, err, "failed to add test serial")

	m, err := sa.GetSerialMetadata(context.Background(), &sapb.Serial{Serial: serial})

	test.AssertNotError(t, err, "getting serial should have succeeded")
	test.AssertEquals(t, m.Serial, serial)
	test.AssertEquals(t, m.RegistrationID, reg.Id)
	test.AssertEquals(t, now, timestamppb.New(now).AsTime())
	test.AssertEquals(t, m.Expires.AsTime(), timestamppb.New(hourLater).AsTime())
}

func TestAddPrecertificate(t *testing.T) {
	ctx := context.Background()
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	// Create a throw-away self signed certificate with a random name and
	// serial number
	serial, testCert := test.ThrowAwayCert(t, clk)

	// Add the cert as a precertificate
	regID := reg.Id
	issuedTime := time.Date(2018, 4, 1, 7, 0, 0, 0, time.UTC)
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        regID,
		Issued:       timestamppb.New(issuedTime),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Couldn't add test cert")

	// It should have the expected certificate status
	certStatus, err := sa.GetCertificateStatus(ctx, &sapb.Serial{Serial: serial})
	test.AssertNotError(t, err, "Couldn't get status for test cert")
	test.AssertEquals(t, certStatus.Status, string(core.OCSPStatusGood))
	now := clk.Now()
	test.AssertEquals(t, now, certStatus.OcspLastUpdated.AsTime())

	// It should show up in the issued names table
	issuedNamesSerial, err := findIssuedName(ctx, sa.dbMap, testCert.DNSNames[0])
	test.AssertNotError(t, err, "expected no err querying issuedNames for precert")
	test.AssertEquals(t, issuedNamesSerial, serial)

	// We should also be able to call AddCertificate with the same cert
	// without it being an error. The duplicate err on inserting to
	// issuedNames should be ignored.
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  regID,
		Issued: timestamppb.New(issuedTime),
	})
	test.AssertNotError(t, err, "unexpected err adding final cert after precert")
}

func TestAddPrecertificateNoOCSP(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)
	_, testCert := test.ThrowAwayCert(t, clk)

	regID := reg.Id
	issuedTime := time.Date(2018, 4, 1, 7, 0, 0, 0, time.UTC)
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        regID,
		Issued:       timestamppb.New(issuedTime),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Couldn't add test cert")
}

func TestAddPreCertificateDuplicate(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	_, testCert := test.ThrowAwayCert(t, clk)
	issuedTime := clk.Now()

	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		Issued:       timestamppb.New(issuedTime),
		RegID:        reg.Id,
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Couldn't add test certificate")

	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		Issued:       timestamppb.New(issuedTime),
		RegID:        reg.Id,
		IssuerNameID: 1,
	})
	test.AssertDeepEquals(t, err, berrors.DuplicateError("cannot add a duplicate cert"))
}

func TestAddPrecertificateIncomplete(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	// Create a throw-away self signed certificate with a random name and
	// serial number
	_, testCert := test.ThrowAwayCert(t, clk)

	// Add the cert as a precertificate
	regID := reg.Id
	issuedTime := time.Date(2018, 4, 1, 7, 0, 0, 0, time.UTC)
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  regID,
		Issued: timestamppb.New(issuedTime),
		// Leaving out IssuerNameID
	})

	test.AssertError(t, err, "Adding precert with no issuer did not fail")
}

func TestAddPrecertificateKeyHash(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()
	reg := createWorkingRegistration(t, sa)

	serial, testCert := test.ThrowAwayCert(t, clk)
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(testCert.NotBefore),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "failed to add precert")

	var keyHashes []keyHashModel
	_, err = sa.dbMap.Select(context.Background(), &keyHashes, "SELECT * FROM keyHashToSerial")
	test.AssertNotError(t, err, "failed to retrieve rows from keyHashToSerial")
	test.AssertEquals(t, len(keyHashes), 1)
	test.AssertEquals(t, keyHashes[0].CertSerial, serial)
	test.AssertEquals(t, keyHashes[0].CertNotAfter, testCert.NotAfter)
	test.AssertEquals(t, keyHashes[0].CertNotAfter, timestamppb.New(testCert.NotAfter).AsTime())
	spkiHash := sha256.Sum256(testCert.RawSubjectPublicKeyInfo)
	test.Assert(t, bytes.Equal(keyHashes[0].KeyHash, spkiHash[:]), "spki hash mismatch")
}

func TestAddCertificate(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	serial, testCert := test.ThrowAwayCert(t, clk)

	issuedTime := sa.clk.Now()
	_, err := sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(issuedTime),
	})
	test.AssertNotError(t, err, "Couldn't add test cert")

	retrievedCert, err := sa.GetCertificate(ctx, &sapb.Serial{Serial: serial})
	test.AssertNotError(t, err, "Couldn't get test cert by full serial")
	test.AssertByteEquals(t, testCert.Raw, retrievedCert.Der)
	test.AssertEquals(t, retrievedCert.Issued.AsTime(), issuedTime)

	// Calling AddCertificate with empty args should fail.
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    nil,
		RegID:  reg.Id,
		Issued: timestamppb.New(issuedTime),
	})
	test.AssertError(t, err, "shouldn't be able to add cert with no DER")
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  0,
		Issued: timestamppb.New(issuedTime),
	})
	test.AssertError(t, err, "shouldn't be able to add cert with no regID")
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: nil,
	})
	test.AssertError(t, err, "shouldn't be able to add cert with no issued timestamp")
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(time.Time{}),
	})
	test.AssertError(t, err, "shouldn't be able to add cert with zero issued timestamp")
}

func TestAddCertificateDuplicate(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	_, testCert := test.ThrowAwayCert(t, clk)

	issuedTime := clk.Now()
	_, err := sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(issuedTime),
	})
	test.AssertNotError(t, err, "Couldn't add test certificate")

	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(issuedTime),
	})
	test.AssertDeepEquals(t, err, berrors.DuplicateError("cannot add a duplicate cert"))

}

func TestCountCertificatesByNamesTimeRange(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)
	_, testCert := test.ThrowAwayCert(t, clk)
	_, err := sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(testCert.NotBefore),
	})
	test.AssertNotError(t, err, "Couldn't add test cert")
	name := testCert.DNSNames[0]

	// Move time forward, so the cert was issued slightly in the past.
	clk.Add(time.Hour)
	now := clk.Now()
	yesterday := clk.Now().Add(-24 * time.Hour)
	twoDaysAgo := clk.Now().Add(-48 * time.Hour)
	tomorrow := clk.Now().Add(24 * time.Hour)

	// Count for a name that doesn't have any certs
	counts, err := sa.CountCertificatesByNames(ctx, &sapb.CountCertificatesByNamesRequest{
		Names: []string{"does.not.exist"},
		Range: &sapb.Range{
			Earliest: timestamppb.New(yesterday),
			Latest:   timestamppb.New(now),
		},
	})
	test.AssertNotError(t, err, "Error counting certs.")
	test.AssertEquals(t, len(counts.Counts), 1)
	test.AssertEquals(t, counts.Counts["does.not.exist"], int64(0))

	// Time range including now should find the cert.
	counts, err = sa.CountCertificatesByNames(ctx, &sapb.CountCertificatesByNamesRequest{
		Names: testCert.DNSNames,
		Range: &sapb.Range{
			Earliest: timestamppb.New(yesterday),
			Latest:   timestamppb.New(now),
		},
	})
	test.AssertNotError(t, err, "sa.CountCertificatesByName failed")
	test.AssertEquals(t, len(counts.Counts), 1)
	test.AssertEquals(t, counts.Counts[name], int64(1))

	// Time range between two days ago and yesterday should not find the cert.
	counts, err = sa.CountCertificatesByNames(ctx, &sapb.CountCertificatesByNamesRequest{
		Names: testCert.DNSNames,
		Range: &sapb.Range{
			Earliest: timestamppb.New(twoDaysAgo),
			Latest:   timestamppb.New(yesterday),
		},
	})
	test.AssertNotError(t, err, "Error counting certs.")
	test.AssertEquals(t, len(counts.Counts), 1)
	test.AssertEquals(t, counts.Counts[name], int64(0))

	// Time range between now and tomorrow also should not (time ranges are
	// inclusive at the tail end, but not the beginning end).
	counts, err = sa.CountCertificatesByNames(ctx, &sapb.CountCertificatesByNamesRequest{
		Names: testCert.DNSNames,
		Range: &sapb.Range{
			Earliest: timestamppb.New(now),
			Latest:   timestamppb.New(tomorrow),
		},
	})
	test.AssertNotError(t, err, "Error counting certs.")
	test.AssertEquals(t, len(counts.Counts), 1)
	test.AssertEquals(t, counts.Counts[name], int64(0))
}

func TestCountCertificatesByNamesParallel(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	// Create two certs with different names and add them both to the database.
	reg := createWorkingRegistration(t, sa)

	_, testCert := test.ThrowAwayCert(t, clk)
	_, err := sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(testCert.NotBefore),
	})
	test.AssertNotError(t, err, "Couldn't add test cert")

	_, testCert2 := test.ThrowAwayCert(t, clk)
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert2.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(testCert2.NotBefore),
	})
	test.AssertNotError(t, err, "Couldn't add test cert")

	// Override countCertificatesByName with an implementation of certCountFunc
	// that will block forever if it's called in serial, but will succeed if
	// called in parallel.
	names := []string{"does.not.exist", testCert.DNSNames[0], testCert2.DNSNames[0]}

	var interlocker sync.WaitGroup
	interlocker.Add(len(names))
	sa.parallelismPerRPC = len(names)
	oldCertCountFunc := sa.countCertificatesByName
	sa.countCertificatesByName = func(ctx context.Context, sel db.Selector, domain string, timeRange *sapb.Range) (int64, time.Time, error) {
		interlocker.Done()
		interlocker.Wait()
		return oldCertCountFunc(ctx, sel, domain, timeRange)
	}

	counts, err := sa.CountCertificatesByNames(ctx, &sapb.CountCertificatesByNamesRequest{
		Names: names,
		Range: &sapb.Range{
			Earliest: timestamppb.New(clk.Now().Add(-time.Hour)),
			Latest:   timestamppb.New(clk.Now().Add(time.Hour)),
		},
	})
	test.AssertNotError(t, err, "Error counting certs.")
	test.AssertEquals(t, len(counts.Counts), 3)

	// We expect there to be two of each of the names that do exist, because
	// test.ThrowAwayCert creates certs for subdomains of example.com, and
	// CountCertificatesByNames counts all certs under the same registered domain.
	expected := map[string]int64{
		"does.not.exist":      0,
		testCert.DNSNames[0]:  2,
		testCert2.DNSNames[0]: 2,
	}
	for name, count := range expected {
		test.AssertEquals(t, count, counts.Counts[name])
	}
}

func TestCountRegistrationsByIP(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	contact := []string{"mailto:foo@example.com"}

	// Create one IPv4 registration
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("43.34.43.34").MarshalText()
	_, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
		Contact:   contact,
	})
	// Create two IPv6 registrations, both within the same /48
	key, _ = jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(2), E: 1}}.MarshalJSON()
	initialIP, _ = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9652").MarshalText()
	test.AssertNotError(t, err, "Couldn't insert registration")
	_, err = sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
		Contact:   contact,
	})
	test.AssertNotError(t, err, "Couldn't insert registration")
	key, _ = jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(3), E: 1}}.MarshalJSON()
	initialIP, _ = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9653").MarshalText()
	_, err = sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
		Contact:   contact,
	})
	test.AssertNotError(t, err, "Couldn't insert registration")

	latest := fc.Now()
	earliest := latest.Add(-time.Hour * 24)
	req := &sapb.CountRegistrationsByIPRequest{
		Ip: net.ParseIP("1.1.1.1"),
		Range: &sapb.Range{
			Earliest: timestamppb.New(earliest),
			Latest:   timestamppb.New(latest),
		},
	}

	// There should be 0 registrations for an IPv4 address we didn't add
	// a registration for
	count, err := sa.CountRegistrationsByIP(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(0))
	// There should be 1 registration for the IPv4 address we did add
	// a registration for.
	req.Ip = net.ParseIP("43.34.43.34")
	count, err = sa.CountRegistrationsByIP(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(1))
	// There should be 1 registration for the first IPv6 address we added
	// a registration for
	req.Ip = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9652")
	count, err = sa.CountRegistrationsByIP(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(1))
	// There should be 1 registration for the second IPv6 address we added
	// a registration for as well
	req.Ip = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9653")
	count, err = sa.CountRegistrationsByIP(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(1))
	// There should be 0 registrations for an IPv6 address in the same /48 as the
	// two IPv6 addresses with registrations
	req.Ip = net.ParseIP("2001:cdba:1234:0000:0000:0000:0000:0000")
	count, err = sa.CountRegistrationsByIP(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(0))
}

func TestCountRegistrationsByIPRange(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	contact := []string{"mailto:foo@example.com"}

	// Create one IPv4 registration
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("43.34.43.34").MarshalText()
	_, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
		Contact:   contact,
	})
	// Create two IPv6 registrations, both within the same /48
	key, _ = jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(2), E: 1}}.MarshalJSON()
	initialIP, _ = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9652").MarshalText()
	test.AssertNotError(t, err, "Couldn't insert registration")
	_, err = sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
		Contact:   contact,
	})
	test.AssertNotError(t, err, "Couldn't insert registration")
	key, _ = jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(3), E: 1}}.MarshalJSON()
	initialIP, _ = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9653").MarshalText()
	_, err = sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
		Contact:   contact,
	})
	test.AssertNotError(t, err, "Couldn't insert registration")

	latest := fc.Now()
	earliest := latest.Add(-time.Hour * 24)
	req := &sapb.CountRegistrationsByIPRequest{
		Ip: net.ParseIP("1.1.1.1"),
		Range: &sapb.Range{
			Earliest: timestamppb.New(earliest),
			Latest:   timestamppb.New(latest),
		},
	}

	// There should be 0 registrations in the range for an IPv4 address we didn't
	// add a registration for
	req.Ip = net.ParseIP("1.1.1.1")
	count, err := sa.CountRegistrationsByIPRange(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(0))
	// There should be 1 registration in the range for the IPv4 address we did
	// add a registration for
	req.Ip = net.ParseIP("43.34.43.34")
	count, err = sa.CountRegistrationsByIPRange(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(1))
	// There should be 2 registrations in the range for the first IPv6 address we added
	// a registration for because it's in the same /48
	req.Ip = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9652")
	count, err = sa.CountRegistrationsByIPRange(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(2))
	// There should be 2 registrations in the range for the second IPv6 address
	// we added a registration for as well, because it too is in the same /48
	req.Ip = net.ParseIP("2001:cdba:1234:5678:9101:1121:3257:9653")
	count, err = sa.CountRegistrationsByIPRange(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(2))
	// There should also be 2 registrations in the range for an arbitrary IPv6 address in
	// the same /48 as the registrations we added
	req.Ip = net.ParseIP("2001:cdba:1234:0000:0000:0000:0000:0000")
	count, err = sa.CountRegistrationsByIPRange(ctx, req)
	test.AssertNotError(t, err, "Failed to count registrations")
	test.AssertEquals(t, count.Count, int64(2))
}

func TestFQDNSets(t *testing.T) {
	ctx := context.Background()
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	tx, err := sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	names := []string{"a.example.com", "B.example.com"}
	expires := fc.Now().Add(time.Hour * 2).UTC()
	issued := fc.Now()
	err = addFQDNSet(ctx, tx, names, "serial", issued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// Invalid Window
	req := &sapb.CountFQDNSetsRequest{
		Domains: names,
		Window:  nil,
	}
	_, err = sa.CountFQDNSets(ctx, req)
	test.AssertErrorIs(t, err, errIncompleteRequest)

	threeHours := time.Hour * 3
	req = &sapb.CountFQDNSetsRequest{
		Domains: names,
		Window:  durationpb.New(threeHours),
	}
	// only one valid
	count, err := sa.CountFQDNSets(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, count.Count, int64(1))

	// check hash isn't affected by changing name order/casing
	req.Domains = []string{"b.example.com", "A.example.COM"}
	count, err = sa.CountFQDNSets(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, count.Count, int64(1))

	// add another valid set
	tx, err = sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	err = addFQDNSet(ctx, tx, names, "anotherSerial", issued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// only two valid
	req.Domains = names
	count, err = sa.CountFQDNSets(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, count.Count, int64(2))

	// add an expired set
	tx, err = sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	err = addFQDNSet(
		ctx,
		tx,
		names,
		"yetAnotherSerial",
		issued.Add(-threeHours),
		expires.Add(-threeHours),
	)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// only two valid
	count, err = sa.CountFQDNSets(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, count.Count, int64(2))
}

func TestFQDNSetTimestampsForWindow(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	tx, err := sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")

	names := []string{"a.example.com", "B.example.com"}

	// Invalid Window
	req := &sapb.CountFQDNSetsRequest{
		Domains: names,
		Window:  nil,
	}
	_, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertErrorIs(t, err, errIncompleteRequest)

	window := time.Hour * 3
	req = &sapb.CountFQDNSetsRequest{
		Domains: names,
		Window:  durationpb.New(window),
	}

	// Ensure zero issuance has occurred for names.
	resp, err := sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 0)

	// Add an issuance for names inside the window.
	expires := fc.Now().Add(time.Hour * 2).UTC()
	firstIssued := fc.Now()
	err = addFQDNSet(ctx, tx, names, "serial", firstIssued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// Ensure there's 1 issuance timestamp for names inside the window.
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 1)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())

	// Ensure that the hash isn't affected by changing name order/casing.
	req.Domains = []string{"b.example.com", "A.example.COM"}
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 1)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())

	// Add another issuance for names inside the window.
	tx, err = sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	err = addFQDNSet(ctx, tx, names, "anotherSerial", firstIssued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// Ensure there are two issuance timestamps for names inside the window.
	req.Domains = names
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 2)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())

	// Add another issuance for names but just outside the window.
	tx, err = sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	err = addFQDNSet(ctx, tx, names, "yetAnotherSerial", firstIssued.Add(-window), expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// Ensure there are still only two issuance timestamps in the window.
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 2)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())
}

func TestFQDNSetsExists(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	names := []string{"a.example.com", "B.example.com"}
	exists, err := sa.FQDNSetExists(ctx, &sapb.FQDNSetExistsRequest{Domains: names})
	test.AssertNotError(t, err, "Failed to check FQDN set existence")
	test.Assert(t, !exists.Exists, "FQDN set shouldn't exist")

	tx, err := sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	expires := fc.Now().Add(time.Hour * 2).UTC()
	issued := fc.Now()
	err = addFQDNSet(ctx, tx, names, "serial", issued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	exists, err = sa.FQDNSetExists(ctx, &sapb.FQDNSetExistsRequest{Domains: names})
	test.AssertNotError(t, err, "Failed to check FQDN set existence")
	test.Assert(t, exists.Exists, "FQDN set does exist")
}

type queryRecorder struct {
	query string
	args  []interface{}
}

func (e *queryRecorder) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	e.query = query
	e.args = args
	return nil, nil
}

func TestAddIssuedNames(t *testing.T) {
	serial := big.NewInt(1)
	expectedSerial := "000000000000000000000000000000000001"
	notBefore := time.Date(2018, 2, 14, 12, 0, 0, 0, time.UTC)
	placeholdersPerName := "(?,?,?,?)"
	baseQuery := "INSERT INTO issuedNames (reversedName,serial,notBefore,renewal) VALUES"

	testCases := []struct {
		Name         string
		IssuedNames  []string
		SerialNumber *big.Int
		NotBefore    time.Time
		Renewal      bool
		ExpectedArgs []interface{}
	}{
		{
			Name:         "One domain, not a renewal",
			IssuedNames:  []string{"example.co.uk"},
			SerialNumber: serial,
			NotBefore:    notBefore,
			Renewal:      false,
			ExpectedArgs: []interface{}{
				"uk.co.example",
				expectedSerial,
				notBefore,
				false,
			},
		},
		{
			Name:         "Two domains, not a renewal",
			IssuedNames:  []string{"example.co.uk", "example.xyz"},
			SerialNumber: serial,
			NotBefore:    notBefore,
			Renewal:      false,
			ExpectedArgs: []interface{}{
				"uk.co.example",
				expectedSerial,
				notBefore,
				false,
				"xyz.example",
				expectedSerial,
				notBefore,
				false,
			},
		},
		{
			Name:         "One domain, renewal",
			IssuedNames:  []string{"example.co.uk"},
			SerialNumber: serial,
			NotBefore:    notBefore,
			Renewal:      true,
			ExpectedArgs: []interface{}{
				"uk.co.example",
				expectedSerial,
				notBefore,
				true,
			},
		},
		{
			Name:         "Two domains, renewal",
			IssuedNames:  []string{"example.co.uk", "example.xyz"},
			SerialNumber: serial,
			NotBefore:    notBefore,
			Renewal:      true,
			ExpectedArgs: []interface{}{
				"uk.co.example",
				expectedSerial,
				notBefore,
				true,
				"xyz.example",
				expectedSerial,
				notBefore,
				true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			var e queryRecorder
			err := addIssuedNames(
				ctx,
				&e,
				&x509.Certificate{
					DNSNames:     tc.IssuedNames,
					SerialNumber: tc.SerialNumber,
					NotBefore:    tc.NotBefore,
				},
				tc.Renewal)
			test.AssertNotError(t, err, "addIssuedNames failed")
			expectedPlaceholders := placeholdersPerName
			for range len(tc.IssuedNames) - 1 {
				expectedPlaceholders = fmt.Sprintf("%s,%s", expectedPlaceholders, placeholdersPerName)
			}
			expectedQuery := fmt.Sprintf("%s %s", baseQuery, expectedPlaceholders)
			test.AssertEquals(t, e.query, expectedQuery)
			if !reflect.DeepEqual(e.args, tc.ExpectedArgs) {
				t.Errorf("Wrong args: got\n%#v, expected\n%#v", e.args, tc.ExpectedArgs)
			}
		})
	}
}

func TestDeactivateAuthorization2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// deactivate a pending authorization
	expires := fc.Now().Add(time.Hour).UTC()
	attemptedAt := fc.Now()
	authzID := createPendingAuthorization(t, sa, "example.com", expires)
	_, err := sa.DeactivateAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "sa.DeactivateAuthorization2 failed")

	// deactivate a valid authorization"
	authzID = createFinalizedAuthorization(t, sa, "example.com", expires, "valid", attemptedAt)
	_, err = sa.DeactivateAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "sa.DeactivateAuthorization2 failed")
}

func TestDeactivateAccount(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	_, err := sa.DeactivateRegistration(context.Background(), &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, "DeactivateRegistration failed")

	dbReg, err := sa.GetRegistration(context.Background(), &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, "GetRegistration failed")
	test.AssertEquals(t, core.AcmeStatus(dbReg.Status), core.StatusDeactivated)
}

func TestReverseName(t *testing.T) {
	testCases := []struct {
		inputDomain   string
		inputReversed string
	}{
		{"", ""},
		{"...", "..."},
		{"com", "com"},
		{"example.com", "com.example"},
		{"www.example.com", "com.example.www"},
		{"world.wide.web.example.com", "com.example.web.wide.world"},
	}

	for _, tc := range testCases {
		output := ReverseName(tc.inputDomain)
		test.AssertEquals(t, output, tc.inputReversed)
	}
}

func TestNewOrderAndAuthzs(t *testing.T) {
	sa, _, cleanup := initSA(t)
	defer cleanup()

	// Create a test registration to reference
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("42.42.42.42").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	// Insert two pre-existing authorizations to reference
	idA := createPendingAuthorization(t, sa, "a.com", sa.clk.Now().Add(time.Hour))
	idB := createPendingAuthorization(t, sa, "b.com", sa.clk.Now().Add(time.Hour))
	test.AssertEquals(t, idA, int64(1))
	test.AssertEquals(t, idB, int64(2))

	nowC := sa.clk.Now().Add(time.Hour)
	nowD := sa.clk.Now().Add(time.Hour)
	expires := sa.clk.Now().Add(2 * time.Hour)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		// Insert an order for four names, two of which already have authzs
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires),
			Names:            []string{"a.com", "b.com", "c.com", "d.com"},
			V2Authorizations: []int64{1, 2},
		},
		// And add new authorizations for the other two names.
		NewAuthzs: []*corepb.Authorization{
			{
				Identifier:     "c.com",
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(nowC),
				Status:         "pending",
				Challenges:     []*corepb.Challenge{{Token: core.NewToken()}},
			},
			{
				Identifier:     "d.com",
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(nowD),
				Status:         "pending",
				Challenges:     []*corepb.Challenge{{Token: core.NewToken()}},
			},
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")
	test.AssertEquals(t, order.Id, int64(1))
	test.AssertDeepEquals(t, order.V2Authorizations, []int64{1, 2, 3, 4})

	var authzIDs []int64
	_, err = sa.dbMap.Select(ctx, &authzIDs, "SELECT authzID FROM orderToAuthz2 WHERE orderID = ?;", order.Id)
	test.AssertNotError(t, err, "Failed to count orderToAuthz entries")
	test.AssertEquals(t, len(authzIDs), 4)
	test.AssertDeepEquals(t, authzIDs, []int64{1, 2, 3, 4})
}

// TestNewOrderAndAuthzs_NonNilInnerOrder verifies that a nil
// sapb.NewOrderAndAuthzsRequest NewOrder object returns an error.
func TestNewOrderAndAuthzs_NonNilInnerOrder(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("17.17.17.17").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	expires := fc.Now().Add(2 * time.Hour)
	_, err = sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewAuthzs: []*corepb.Authorization{
			{
				Identifier:     "a.com",
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(expires),
				Status:         "pending",
				Challenges:     []*corepb.Challenge{{Token: core.NewToken()}},
			},
		},
	})
	test.AssertErrorIs(t, err, errIncompleteRequest)
}

func TestNewOrderAndAuthzs_NewAuthzExpectedFields(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	// Create a test registration to reference.
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("17.17.17.17").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	expires := fc.Now().Add(time.Hour)
	domain := "a.com"

	// Create an authz that does not yet exist in the database with some invalid
	// data smuggled in.
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewAuthzs: []*corepb.Authorization{
			{
				Identifier:     domain,
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(expires),
				Status:         string(core.StatusPending),
				Challenges: []*corepb.Challenge{
					{
						Status: "real fake garbage data",
						Token:  core.NewToken(),
					},
				},
			},
		},
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID: reg.Id,
			Expires:        timestamppb.New(expires),
			Names:          []string{domain},
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	// Safely get the authz for the order we created above.
	obj, err := sa.dbReadOnlyMap.Get(ctx, authzModel{}, order.V2Authorizations[0])
	test.AssertNotError(t, err, fmt.Sprintf("authorization %d not found", order.V2Authorizations[0]))

	// To access the data stored in obj at compile time, we type assert obj
	// into a pointer to an authzModel.
	am, ok := obj.(*authzModel)
	test.Assert(t, ok, "Could not type assert obj into authzModel")

	// If we're making a brand new authz, it should have the pending status
	// regardless of what incorrect status value was passed in during construction.
	test.AssertEquals(t, am.Status, statusUint(core.StatusPending))

	// Testing for the existence of these boxed nils is a definite break from
	// our paradigm of avoiding passing around boxed nils whenever possible.
	// However, the existence of these boxed nils in relation to this test is
	// actually expected. If these tests fail, then a possible SA refactor or RA
	// bug placed incorrect data into brand new authz input fields.
	test.AssertBoxedNil(t, am.Attempted, "am.Attempted should be nil")
	test.AssertBoxedNil(t, am.AttemptedAt, "am.AttemptedAt should be nil")
	test.AssertBoxedNil(t, am.ValidationError, "am.ValidationError should be nil")
	test.AssertBoxedNil(t, am.ValidationRecord, "am.ValidationRecord should be nil")
}

func TestSetOrderProcessing(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	// Create a test registration to reference
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("42.42.42.42").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	// Add one valid authz
	expires := fc.Now().Add(time.Hour)
	attemptedAt := fc.Now()
	authzID := createFinalizedAuthorization(t, sa, "example.com", expires, "valid", attemptedAt)

	// Add a new order in pending status with no certificate serial
	expires1Year := sa.clk.Now().Add(365 * 24 * time.Hour)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Names:            []string{"example.com"},
			V2Authorizations: []int64{authzID},
		},
	})
	test.AssertNotError(t, err, "NewOrderAndAuthzs failed")

	// Set the order to be processing
	_, err = sa.SetOrderProcessing(context.Background(), &sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "SetOrderProcessing failed")

	// Read the order by ID from the DB to check the status was correctly updated
	// to processing
	updatedOrder, err := sa.GetOrder(
		context.Background(),
		&sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "GetOrder failed")
	test.AssertEquals(t, updatedOrder.Status, string(core.StatusProcessing))
	test.AssertEquals(t, updatedOrder.BeganProcessing, true)

	// Try to set the same order to be processing again. We should get an error.
	_, err = sa.SetOrderProcessing(context.Background(), &sapb.OrderRequest{Id: order.Id})
	test.AssertError(t, err, "Set the same order processing twice. This should have been an error.")
	test.AssertErrorIs(t, err, berrors.OrderNotReady)
}

func TestFinalizeOrder(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	// Create a test registration to reference
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("42.42.42.42").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	// Add one valid authz
	expires := fc.Now().Add(time.Hour)
	attemptedAt := fc.Now()
	authzID := createFinalizedAuthorization(t, sa, "example.com", expires, "valid", attemptedAt)

	// Add a new order in pending status with no certificate serial
	expires1Year := sa.clk.Now().Add(365 * 24 * time.Hour)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Names:            []string{"example.com"},
			V2Authorizations: []int64{authzID},
		},
	})
	test.AssertNotError(t, err, "NewOrderAndAuthzs failed")

	// Set the order to processing so it can be finalized
	_, err = sa.SetOrderProcessing(ctx, &sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "SetOrderProcessing failed")

	// Finalize the order with a certificate serial
	order.CertificateSerial = "eat.serial.for.breakfast"
	_, err = sa.FinalizeOrder(context.Background(), &sapb.FinalizeOrderRequest{Id: order.Id, CertificateSerial: order.CertificateSerial})
	test.AssertNotError(t, err, "FinalizeOrder failed")

	// Read the order by ID from the DB to check the certificate serial and status
	// was correctly updated
	updatedOrder, err := sa.GetOrder(
		context.Background(),
		&sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "GetOrder failed")
	test.AssertEquals(t, updatedOrder.CertificateSerial, "eat.serial.for.breakfast")
	test.AssertEquals(t, updatedOrder.Status, string(core.StatusValid))
}

func TestOrderWithOrderModelv1(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	// Create a test registration to reference
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("42.42.42.42").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	authzExpires := fc.Now().Add(time.Hour)
	authzID := createPendingAuthorization(t, sa, "example.com", authzExpires)

	// Set the order to expire in two hours
	expires := fc.Now().Add(2 * time.Hour)

	inputOrder := &corepb.Order{
		RegistrationID:   reg.Id,
		Expires:          timestamppb.New(expires),
		Names:            []string{"example.com"},
		V2Authorizations: []int64{authzID},
	}

	// Create the order
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   inputOrder.RegistrationID,
			Expires:          inputOrder.Expires,
			Names:            inputOrder.Names,
			V2Authorizations: inputOrder.V2Authorizations,
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	// The Order from GetOrder should match the following expected order
	created := sa.clk.Now()
	expectedOrder := &corepb.Order{
		// The registration ID, authorizations, expiry, and names should match the
		// input to NewOrderAndAuthzs
		RegistrationID:   inputOrder.RegistrationID,
		V2Authorizations: inputOrder.V2Authorizations,
		Names:            inputOrder.Names,
		Expires:          inputOrder.Expires,
		// The ID should have been set to 1 by the SA
		Id: 1,
		// The status should be pending
		Status: string(core.StatusPending),
		// The serial should be empty since this is a pending order
		CertificateSerial: "",
		// We should not be processing it
		BeganProcessing: false,
		// The created timestamp should have been set to the current time
		Created: timestamppb.New(created),
	}

	// Fetch the order by its ID and make sure it matches the expected
	storedOrder, err := sa.GetOrder(context.Background(), &sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "sa.GetOrder failed")
	test.AssertDeepEquals(t, storedOrder, expectedOrder)
}

func TestOrderWithOrderModelv2(t *testing.T) {
	if !strings.Contains(os.Getenv("BOULDER_CONFIG_DIR"), "test/config-next") {
		t.Skip()
	}

	// The feature must be set before the SA is constructed because of a
	// conditional on this feature in //sa/database.go.
	features.Set(features.Config{MultipleCertificateProfiles: true})
	defer features.Reset()

	fc := clock.NewFake()
	fc.Set(time.Date(2015, 3, 4, 5, 0, 0, 0, time.UTC))

	dbMap, err := DBMapForTest(vars.DBConnSA)
	test.AssertNotError(t, err, "Couldn't create dbMap")

	saro, err := NewSQLStorageAuthorityRO(dbMap, nil, metrics.NoopRegisterer, 1, 0, fc, log)
	test.AssertNotError(t, err, "Couldn't create SARO")

	sa, err := NewSQLStorageAuthorityWrapping(saro, dbMap, metrics.NoopRegisterer)
	test.AssertNotError(t, err, "Couldn't create SA")
	defer test.ResetBoulderTestDatabase(t)

	// Create a test registration to reference
	key, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1), E: 1}}.MarshalJSON()
	initialIP, _ := net.ParseIP("42.42.42.42").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	authzExpires := fc.Now().Add(time.Hour)
	authzID := createPendingAuthorization(t, sa, "example.com", authzExpires)

	// Set the order to expire in two hours
	expires := fc.Now().Add(2 * time.Hour)

	inputOrder := &corepb.Order{
		RegistrationID:         reg.Id,
		Expires:                timestamppb.New(expires),
		Names:                  []string{"example.com"},
		V2Authorizations:       []int64{authzID},
		CertificateProfileName: "tbiapb",
	}

	// Create the order
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:         inputOrder.RegistrationID,
			Expires:                inputOrder.Expires,
			Names:                  inputOrder.Names,
			V2Authorizations:       inputOrder.V2Authorizations,
			CertificateProfileName: inputOrder.CertificateProfileName,
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	// The Order from GetOrder should match the following expected order
	created := sa.clk.Now()
	expectedOrder := &corepb.Order{
		// The registration ID, authorizations, expiry, and names should match the
		// input to NewOrderAndAuthzs
		RegistrationID:   inputOrder.RegistrationID,
		V2Authorizations: inputOrder.V2Authorizations,
		Names:            inputOrder.Names,
		Expires:          inputOrder.Expires,
		// The ID should have been set to 1 by the SA
		Id: 1,
		// The status should be pending
		Status: string(core.StatusPending),
		// The serial should be empty since this is a pending order
		CertificateSerial: "",
		// We should not be processing it
		BeganProcessing: false,
		// The created timestamp should have been set to the current time
		Created:                timestamppb.New(created),
		CertificateProfileName: "tbiapb",
	}

	// Fetch the order by its ID and make sure it matches the expected
	storedOrder, err := sa.GetOrder(context.Background(), &sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "sa.GetOrder failed")
	test.AssertDeepEquals(t, storedOrder, expectedOrder)

	//
	// Test that an order without a certificate profile name, but with the
	// MultipleCertificateProfiles feature flag enabled works as expected.
	//

	// Create a test registration to reference
	key2, _ := jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(2), E: 2}}.MarshalJSON()
	initialIP2, _ := net.ParseIP("44.44.44.44").MarshalText()
	reg2, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key2,
		InitialIP: initialIP2,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	inputOrderNoName := &corepb.Order{
		RegistrationID:   reg2.Id,
		Expires:          timestamppb.New(expires),
		Names:            []string{"example.com"},
		V2Authorizations: []int64{authzID},
	}

	// Create the order
	orderNoName, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:         inputOrderNoName.RegistrationID,
			Expires:                inputOrderNoName.Expires,
			Names:                  inputOrderNoName.Names,
			V2Authorizations:       inputOrderNoName.V2Authorizations,
			CertificateProfileName: inputOrderNoName.CertificateProfileName,
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	// The Order from GetOrder should match the following expected order
	created = sa.clk.Now()
	expectedOrderNoName := &corepb.Order{
		// The registration ID, authorizations, expiry, and names should match the
		// input to NewOrderAndAuthzs
		RegistrationID:   inputOrderNoName.RegistrationID,
		V2Authorizations: inputOrderNoName.V2Authorizations,
		Names:            inputOrderNoName.Names,
		Expires:          inputOrderNoName.Expires,
		// The ID should have been set to 2 by the SA
		Id: 2,
		// The status should be pending
		Status: string(core.StatusPending),
		// The serial should be empty since this is a pending order
		CertificateSerial: "",
		// We should not be processing it
		BeganProcessing: false,
		// The created timestamp should have been set to the current time
		Created: timestamppb.New(created),
	}

	// Fetch the order by its ID and make sure it matches the expected
	storedOrderNoName, err := sa.GetOrder(context.Background(), &sapb.OrderRequest{Id: orderNoName.Id})
	test.AssertNotError(t, err, "sa.GetOrder failed")
	test.AssertDeepEquals(t, storedOrderNoName, expectedOrderNoName)
}

// TestGetAuthorization2NoRows ensures that the GetAuthorization2 function returns
// the correct error when there are no results for the provided ID.
func TestGetAuthorization2NoRows(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	// An empty authz ID should result in a not found berror.
	id := int64(123)
	_, err := sa.GetAuthorization2(ctx, &sapb.AuthorizationID2{Id: id})
	test.AssertError(t, err, "Didn't get an error looking up non-existent authz ID")
	test.AssertErrorIs(t, err, berrors.NotFound)
}

func TestGetAuthorizations2(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	reg := createWorkingRegistration(t, sa)
	exp := fc.Now().AddDate(0, 0, 10).UTC()
	attemptedAt := fc.Now()

	identA := "aaa"
	identB := "bbb"
	identC := "ccc"
	identD := "ddd"
	idents := []string{identA, identB, identC}

	authzIDA := createFinalizedAuthorization(t, sa, "aaa", exp, "valid", attemptedAt)
	authzIDB := createPendingAuthorization(t, sa, "bbb", exp)
	nearbyExpires := fc.Now().UTC().Add(time.Hour)
	authzIDC := createPendingAuthorization(t, sa, "ccc", nearbyExpires)

	// Associate authorizations with an order so that GetAuthorizations2 thinks
	// they are WFE2 authorizations.
	err := sa.dbMap.Insert(ctx, &orderToAuthzModel{
		OrderID: 1,
		AuthzID: authzIDA,
	})
	test.AssertNotError(t, err, "sa.dbMap.Insert failed")
	err = sa.dbMap.Insert(ctx, &orderToAuthzModel{
		OrderID: 1,
		AuthzID: authzIDB,
	})
	test.AssertNotError(t, err, "sa.dbMap.Insert failed")
	err = sa.dbMap.Insert(ctx, &orderToAuthzModel{
		OrderID: 1,
		AuthzID: authzIDC,
	})
	test.AssertNotError(t, err, "sa.dbMap.Insert failed")

	// Set an expiry cut off of 1 day in the future similar to `RA.NewOrderAndAuthzs`. This
	// should exclude pending authorization C based on its nearbyExpires expiry
	// value.
	expiryCutoff := fc.Now().AddDate(0, 0, 1)
	// Get authorizations for the names used above.
	authz, err := sa.GetAuthorizations2(context.Background(), &sapb.GetAuthorizationsRequest{
		RegistrationID: reg.Id,
		Domains:        idents,
		Now:            timestamppb.New(expiryCutoff),
	})
	// It should not fail
	test.AssertNotError(t, err, "sa.GetAuthorizations2 failed")
	// We should get back two authorizations since one of the three authorizations
	// created above expires too soon.
	test.AssertEquals(t, len(authz.Authz), 2)

	// Get authorizations for the names used above, and one name that doesn't exist
	authz, err = sa.GetAuthorizations2(context.Background(), &sapb.GetAuthorizationsRequest{
		RegistrationID: reg.Id,
		Domains:        append(idents, identD),
		Now:            timestamppb.New(expiryCutoff),
	})
	// It should not fail
	test.AssertNotError(t, err, "sa.GetAuthorizations2 failed")
	// It should still return only two authorizations
	test.AssertEquals(t, len(authz.Authz), 2)
}

func TestCountOrders(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)
	now := sa.clk.Now()
	expires := now.Add(24 * time.Hour)

	req := &sapb.CountOrdersRequest{
		AccountID: 12345,
		Range: &sapb.Range{
			Earliest: timestamppb.New(now.Add(-time.Hour)),
			Latest:   timestamppb.New(now.Add(time.Second)),
		},
	}

	// Counting new orders for a reg ID that doesn't exist should return 0
	count, err := sa.CountOrders(ctx, req)
	test.AssertNotError(t, err, "Couldn't count new orders for fake reg ID")
	test.AssertEquals(t, count.Count, int64(0))

	// Add a pending authorization
	authzID := createPendingAuthorization(t, sa, "example.com", expires)

	// Add one pending order
	order, err := sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires),
			Names:            []string{"example.com"},
			V2Authorizations: []int64{authzID},
		},
	})
	test.AssertNotError(t, err, "Couldn't create new pending order")

	// Counting new orders for the reg ID should now yield 1
	req.AccountID = reg.Id
	count, err = sa.CountOrders(ctx, req)
	test.AssertNotError(t, err, "Couldn't count new orders for reg ID")
	test.AssertEquals(t, count.Count, int64(1))

	// Moving the count window to after the order was created should return the
	// count to 0
	earliest := order.Created.AsTime().Add(time.Minute)
	latest := earliest.Add(time.Hour)
	req.Range.Earliest = timestamppb.New(earliest)
	req.Range.Latest = timestamppb.New(latest)
	count, err = sa.CountOrders(ctx, req)
	test.AssertNotError(t, err, "Couldn't count new orders for reg ID")
	test.AssertEquals(t, count.Count, int64(0))
}

func TestFasterGetOrderForNames(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	domain := "example.com"
	expires := fc.Now().Add(time.Hour)

	key, _ := goodTestJWK().MarshalJSON()
	initialIP, _ := net.ParseIP("42.42.42.42").MarshalText()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	authzIDs := createPendingAuthorization(t, sa, domain, expires)

	_, err = sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires),
			V2Authorizations: []int64{authzIDs},
			Names:            []string{domain},
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	_, err = sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires),
			V2Authorizations: []int64{authzIDs},
			Names:            []string{domain},
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	_, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: reg.Id,
		Names:  []string{domain},
	})
	test.AssertNotError(t, err, "sa.GetOrderForNames failed")
}

func TestGetOrderForNames(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Give the order we create a short lifetime
	orderLifetime := time.Hour
	expires := fc.Now().Add(orderLifetime)

	// Create two test registrations to associate with orders
	key, _ := goodTestJWK().MarshalJSON()
	initialIP, _ := net.ParseIP("42.42.42.42").MarshalText()
	regA, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:       key,
		InitialIP: initialIP,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	// Add one pending authz for the first name for regA and one
	// pending authz for the second name for regA
	authzExpires := fc.Now().Add(time.Hour)
	authzIDA := createPendingAuthorization(t, sa, "example.com", authzExpires)
	authzIDB := createPendingAuthorization(t, sa, "just.another.example.com", authzExpires)

	ctx := context.Background()
	names := []string{"example.com", "just.another.example.com"}

	// Call GetOrderForNames for a set of names we haven't created an order for
	// yet
	result, err := sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: regA.Id,
		Names:  names,
	})
	// We expect the result to return an error
	test.AssertError(t, err, "sa.GetOrderForNames did not return an error for an empty result")
	// The error should be a notfound error
	test.AssertErrorIs(t, err, berrors.NotFound)
	// The result should be nil
	test.Assert(t, result == nil, "sa.GetOrderForNames for non-existent order returned non-nil result")

	// Add a new order for a set of names
	order, err := sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   regA.Id,
			Expires:          timestamppb.New(expires),
			V2Authorizations: []int64{authzIDA, authzIDB},
			Names:            names,
		},
	})
	// It shouldn't error
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")
	// The order ID shouldn't be nil
	test.AssertNotNil(t, order.Id, "NewOrderAndAuthzs returned with a nil Id")

	// Call GetOrderForNames with the same account ID and set of names as the
	// above NewOrderAndAuthzs call
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: regA.Id,
		Names:  names,
	})
	// It shouldn't error
	test.AssertNotError(t, err, "sa.GetOrderForNames failed")
	// The order returned should have the same ID as the order we created above
	test.AssertNotNil(t, result, "Returned order was nil")
	test.AssertEquals(t, result.Id, order.Id)

	// Call GetOrderForNames with a different account ID from the NewOrderAndAuthzs call
	regB := int64(1337)
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: regB,
		Names:  names,
	})
	// It should error
	test.AssertError(t, err, "sa.GetOrderForNames did not return an error for an empty result")
	// The error should be a notfound error
	test.AssertErrorIs(t, err, berrors.NotFound)
	// The result should be nil
	test.Assert(t, result == nil, "sa.GetOrderForNames for diff AcctID returned non-nil result")

	// Advance the clock beyond the initial order's lifetime
	fc.Add(2 * orderLifetime)

	// Call GetOrderForNames again with the same account ID and set of names as
	// the initial NewOrderAndAuthzs call
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: regA.Id,
		Names:  names,
	})
	// It should error since there is no result
	test.AssertError(t, err, "sa.GetOrderForNames did not return an error for an empty result")
	// The error should be a notfound error
	test.AssertErrorIs(t, err, berrors.NotFound)
	// The result should be nil because the initial order expired & we don't want
	// to return expired orders
	test.Assert(t, result == nil, "sa.GetOrderForNames returned non-nil result for expired order case")

	// Create two valid authorizations
	authzExpires = fc.Now().Add(time.Hour)
	attemptedAt := fc.Now()
	authzIDC := createFinalizedAuthorization(t, sa, "zombo.com", authzExpires, "valid", attemptedAt)
	authzIDD := createFinalizedAuthorization(t, sa, "welcome.to.zombo.com", authzExpires, "valid", attemptedAt)

	// Add a fresh order that uses the authorizations created above
	names = []string{"zombo.com", "welcome.to.zombo.com"}
	expires = fc.Now().Add(orderLifetime)
	order, err = sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   regA.Id,
			Expires:          timestamppb.New(expires),
			V2Authorizations: []int64{authzIDC, authzIDD},
			Names:            names,
		},
	})
	// It shouldn't error
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")
	// The order ID shouldn't be nil
	test.AssertNotNil(t, order.Id, "NewOrderAndAuthzs returned with a nil Id")

	// Call GetOrderForNames with the same account ID and set of names as
	// the earlier NewOrderAndAuthzs call
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: regA.Id,
		Names:  names,
	})
	// It should not error since a ready order can be reused.
	test.AssertNotError(t, err, "sa.GetOrderForNames returned an unexpected error for ready order reuse")
	// The order returned should have the same ID as the order we created above
	test.AssertNotNil(t, result, "sa.GetOrderForNames returned nil result")
	test.AssertEquals(t, result.Id, order.Id)

	// Set the order processing so it can be finalized
	_, err = sa.SetOrderProcessing(ctx, &sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "sa.SetOrderProcessing failed")

	// Finalize the order
	order.CertificateSerial = "cinnamon toast crunch"
	_, err = sa.FinalizeOrder(ctx, &sapb.FinalizeOrderRequest{Id: order.Id, CertificateSerial: order.CertificateSerial})
	test.AssertNotError(t, err, "sa.FinalizeOrder failed")

	// Call GetOrderForNames with the same account ID and set of names as
	// the earlier NewOrderAndAuthzs call
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: regA.Id,
		Names:  names,
	})
	// It should error since a valid order should not be reused.
	test.AssertError(t, err, "sa.GetOrderForNames did not return an error for an empty result")
	// The error should be a notfound error
	test.AssertErrorIs(t, err, berrors.NotFound)
	// The result should be nil because the one matching order has been finalized
	// already
	test.Assert(t, result == nil, "sa.GetOrderForNames returned non-nil result for finalized order case")
}

func TestStatusForOrder(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	ctx := context.Background()
	expires := fc.Now().Add(time.Hour)
	alreadyExpired := expires.Add(-2 * time.Hour)
	attemptedAt := fc.Now()

	// Create a registration to work with
	reg := createWorkingRegistration(t, sa)

	// Create a pending authz, an expired authz, an invalid authz, a deactivated authz,
	// and a valid authz
	pendingID := createPendingAuthorization(t, sa, "pending.your.order.is.up", expires)
	expiredID := createPendingAuthorization(t, sa, "expired.your.order.is.up", alreadyExpired)
	invalidID := createFinalizedAuthorization(t, sa, "invalid.your.order.is.up", expires, "invalid", attemptedAt)
	validID := createFinalizedAuthorization(t, sa, "valid.your.order.is.up", expires, "valid", attemptedAt)
	deactivatedID := createPendingAuthorization(t, sa, "deactivated.your.order.is.up", expires)
	_, err := sa.DeactivateAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: deactivatedID})
	test.AssertNotError(t, err, "sa.DeactivateAuthorization2 failed")

	testCases := []struct {
		Name             string
		AuthorizationIDs []int64
		OrderNames       []string
		OrderExpires     *timestamppb.Timestamp
		ExpectedStatus   string
		SetProcessing    bool
		Finalize         bool
	}{
		{
			Name:             "Order with an invalid authz",
			OrderNames:       []string{"pending.your.order.is.up", "invalid.your.order.is.up", "deactivated.your.order.is.up", "valid.your.order.is.up"},
			AuthorizationIDs: []int64{pendingID, invalidID, deactivatedID, validID},
			ExpectedStatus:   string(core.StatusInvalid),
		},
		{
			Name:             "Order with an expired authz",
			OrderNames:       []string{"pending.your.order.is.up", "expired.your.order.is.up", "deactivated.your.order.is.up", "valid.your.order.is.up"},
			AuthorizationIDs: []int64{pendingID, expiredID, deactivatedID, validID},
			ExpectedStatus:   string(core.StatusInvalid),
		},
		{
			Name:             "Order with a deactivated authz",
			OrderNames:       []string{"pending.your.order.is.up", "deactivated.your.order.is.up", "valid.your.order.is.up"},
			AuthorizationIDs: []int64{pendingID, deactivatedID, validID},
			ExpectedStatus:   string(core.StatusInvalid),
		},
		{
			Name:             "Order with a pending authz",
			OrderNames:       []string{"valid.your.order.is.up", "pending.your.order.is.up"},
			AuthorizationIDs: []int64{validID, pendingID},
			ExpectedStatus:   string(core.StatusPending),
		},
		{
			Name:             "Order with only valid authzs, not yet processed or finalized",
			OrderNames:       []string{"valid.your.order.is.up"},
			AuthorizationIDs: []int64{validID},
			ExpectedStatus:   string(core.StatusReady),
		},
		{
			Name:             "Order with only valid authzs, set processing",
			OrderNames:       []string{"valid.your.order.is.up"},
			AuthorizationIDs: []int64{validID},
			SetProcessing:    true,
			ExpectedStatus:   string(core.StatusProcessing),
		},
		{
			Name:             "Order with only valid authzs, not yet processed or finalized, OrderReadyStatus feature flag",
			OrderNames:       []string{"valid.your.order.is.up"},
			AuthorizationIDs: []int64{validID},
			ExpectedStatus:   string(core.StatusReady),
		},
		{
			Name:             "Order with only valid authzs, set processing",
			OrderNames:       []string{"valid.your.order.is.up"},
			AuthorizationIDs: []int64{validID},
			SetProcessing:    true,
			ExpectedStatus:   string(core.StatusProcessing),
		},
		{
			Name:             "Order with only valid authzs, set processing and finalized",
			OrderNames:       []string{"valid.your.order.is.up"},
			AuthorizationIDs: []int64{validID},
			SetProcessing:    true,
			Finalize:         true,
			ExpectedStatus:   string(core.StatusValid),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// If the testcase doesn't specify an order expiry use a default timestamp
			// in the near future.
			orderExpiry := tc.OrderExpires
			if !orderExpiry.IsValid() {
				orderExpiry = timestamppb.New(expires)
			}

			newOrder, err := sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
				NewOrder: &sapb.NewOrderRequest{
					RegistrationID:   reg.Id,
					Expires:          orderExpiry,
					V2Authorizations: tc.AuthorizationIDs,
					Names:            tc.OrderNames,
				},
			})
			test.AssertNotError(t, err, "NewOrderAndAuthzs errored unexpectedly")
			// If requested, set the order to processing
			if tc.SetProcessing {
				_, err := sa.SetOrderProcessing(ctx, &sapb.OrderRequest{Id: newOrder.Id})
				test.AssertNotError(t, err, "Error setting order to processing status")
			}
			// If requested, finalize the order
			if tc.Finalize {
				newOrder.CertificateSerial = "lucky charms"
				_, err = sa.FinalizeOrder(ctx, &sapb.FinalizeOrderRequest{Id: newOrder.Id, CertificateSerial: newOrder.CertificateSerial})
				test.AssertNotError(t, err, "Error finalizing order")
			}
			// Fetch the order by ID to get its calculated status
			storedOrder, err := sa.GetOrder(ctx, &sapb.OrderRequest{Id: newOrder.Id})
			test.AssertNotError(t, err, "GetOrder failed")
			// The status shouldn't be nil
			test.AssertNotNil(t, storedOrder.Status, "Order status was nil")
			// The status should match expected
			test.AssertEquals(t, storedOrder.Status, tc.ExpectedStatus)
		})
	}

}

func TestUpdateChallengesDeleteUnused(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	expires := fc.Now().Add(time.Hour)
	ctx := context.Background()
	attemptedAt := fc.Now()

	// Create a valid authz
	authzID := createFinalizedAuthorization(t, sa, "example.com", expires, "valid", attemptedAt)

	result, err := sa.GetAuthorization2(ctx, &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "sa.GetAuthorization2 failed")

	if len(result.Challenges) != 1 {
		t.Fatalf("expected 1 challenge left after finalization, got %d", len(result.Challenges))
	}
	if result.Challenges[0].Status != string(core.StatusValid) {
		t.Errorf("expected challenge status %q, got %q", core.StatusValid, result.Challenges[0].Status)
	}
	if result.Challenges[0].Type != "http-01" {
		t.Errorf("expected challenge type %q, got %q", "http-01", result.Challenges[0].Type)
	}
}

func TestRevokeCertificate(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)
	// Add a cert to the DB to test with.
	serial, testCert := test.ThrowAwayCert(t, fc)
	issuedTime := sa.clk.Now()
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(issuedTime),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Couldn't add test cert")

	status, err := sa.GetCertificateStatus(ctx, &sapb.Serial{Serial: serial})
	test.AssertNotError(t, err, "GetCertificateStatus failed")
	test.AssertEquals(t, core.OCSPStatus(status.Status), core.OCSPStatusGood)

	fc.Add(1 * time.Hour)

	now := fc.Now()
	reason := int64(1)

	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Reason:   reason,
	})
	test.AssertNotError(t, err, "RevokeCertificate with no OCSP response should succeed")

	status, err = sa.GetCertificateStatus(ctx, &sapb.Serial{Serial: serial})
	test.AssertNotError(t, err, "GetCertificateStatus failed")
	test.AssertEquals(t, core.OCSPStatus(status.Status), core.OCSPStatusRevoked)
	test.AssertEquals(t, status.RevokedReason, reason)
	test.AssertEquals(t, status.RevokedDate.AsTime(), now)
	test.AssertEquals(t, status.OcspLastUpdated.AsTime(), now)

	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Reason:   reason,
	})
	test.AssertError(t, err, "RevokeCertificate should've failed when certificate already revoked")
}

func TestRevokeCertificateWithShard(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires revokedCertificates database table")
	}

	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Add a cert to the DB to test with.
	reg := createWorkingRegistration(t, sa)
	eeCert, err := core.LoadCert("../test/hierarchy/ee-e1.cert.pem")
	test.AssertNotError(t, err, "failed to load test cert")
	_, err = sa.AddSerial(ctx, &sapb.AddSerialRequest{
		RegID:   reg.Id,
		Serial:  core.SerialToString(eeCert.SerialNumber),
		Created: timestamppb.New(eeCert.NotBefore),
		Expires: timestamppb.New(eeCert.NotAfter),
	})
	test.AssertNotError(t, err, "failed to add test serial")
	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          eeCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(eeCert.NotBefore),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "failed to add test cert")

	serial := core.SerialToString(eeCert.SerialNumber)
	fc.Add(1 * time.Hour)
	now := fc.Now()
	reason := int64(1)

	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		ShardIdx: 9,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Reason:   reason,
	})
	test.AssertNotError(t, err, "RevokeCertificate with no OCSP response should succeed")

	status, err := sa.GetCertificateStatus(ctx, &sapb.Serial{Serial: serial})
	test.AssertNotError(t, err, "GetCertificateStatus failed")
	test.AssertEquals(t, core.OCSPStatus(status.Status), core.OCSPStatusRevoked)
	test.AssertEquals(t, status.RevokedReason, reason)
	test.AssertEquals(t, status.RevokedDate.AsTime(), now)
	test.AssertEquals(t, status.OcspLastUpdated.AsTime(), now)
	test.AssertEquals(t, status.NotAfter.AsTime(), eeCert.NotAfter)

	var result revokedCertModel
	err = sa.dbMap.SelectOne(
		ctx, &result, `SELECT * FROM revokedCertificates WHERE serial = ?`, core.SerialToString(eeCert.SerialNumber))
	test.AssertNotError(t, err, "should be exactly one row in revokedCertificates")
	test.AssertEquals(t, result.ShardIdx, int64(9))
	test.AssertEquals(t, result.RevokedReason, revocation.Reason(ocsp.KeyCompromise))
}

func TestUpdateRevokedCertificate(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Add a cert to the DB to test with.
	reg := createWorkingRegistration(t, sa)
	serial, testCert := test.ThrowAwayCert(t, fc)
	issuedTime := fc.Now()
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(issuedTime),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Couldn't add test cert")
	fc.Add(1 * time.Hour)

	// Try to update it before its been revoked
	now := fc.Now()
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Backdate: timestamppb.New(now),
		Reason:   ocsp.KeyCompromise,
		Response: []byte{4, 5, 6},
	})
	test.AssertError(t, err, "UpdateRevokedCertificate should have failed")
	test.AssertContains(t, err.Error(), "no certificate with serial")

	// Now revoke it, so we can update it.
	revokedTime := fc.Now()
	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(revokedTime),
		Reason:   ocsp.CessationOfOperation,
		Response: []byte{1, 2, 3},
	})
	test.AssertNotError(t, err, "RevokeCertificate failed")

	// Double check that setup worked.
	status, err := sa.GetCertificateStatus(ctx, &sapb.Serial{Serial: serial})
	test.AssertNotError(t, err, "GetCertificateStatus failed")
	test.AssertEquals(t, core.OCSPStatus(status.Status), core.OCSPStatusRevoked)
	test.AssertEquals(t, int(status.RevokedReason), ocsp.CessationOfOperation)
	fc.Add(1 * time.Hour)

	// Try to update its revocation info with no backdate
	now = fc.Now()
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Reason:   ocsp.KeyCompromise,
		Response: []byte{4, 5, 6},
	})
	test.AssertError(t, err, "UpdateRevokedCertificate should have failed")
	test.AssertContains(t, err.Error(), "incomplete")

	// Try to update its revocation info for a reason other than keyCompromise
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Backdate: timestamppb.New(revokedTime),
		Reason:   ocsp.Unspecified,
		Response: []byte{4, 5, 6},
	})
	test.AssertError(t, err, "UpdateRevokedCertificate should have failed")
	test.AssertContains(t, err.Error(), "cannot update revocation for any reason other than keyCompromise")

	// Try to update the revocation info of the wrong certificate
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   "000000000000000000000000000000021bd5",
		Date:     timestamppb.New(now),
		Backdate: timestamppb.New(revokedTime),
		Reason:   ocsp.KeyCompromise,
		Response: []byte{4, 5, 6},
	})
	test.AssertError(t, err, "UpdateRevokedCertificate should have failed")
	test.AssertContains(t, err.Error(), "no certificate with serial")

	// Try to update its revocation info with the wrong backdate
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Backdate: timestamppb.New(now),
		Reason:   ocsp.KeyCompromise,
		Response: []byte{4, 5, 6},
	})
	test.AssertError(t, err, "UpdateRevokedCertificate should have failed")
	test.AssertContains(t, err.Error(), "no certificate with serial")

	// Try to update its revocation info correctly
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     timestamppb.New(now),
		Backdate: timestamppb.New(revokedTime),
		Reason:   ocsp.KeyCompromise,
		Response: []byte{4, 5, 6},
	})
	test.AssertNotError(t, err, "UpdateRevokedCertificate failed")
}

func TestUpdateRevokedCertificateWithShard(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires revokedCertificates database table")
	}

	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Add a cert to the DB to test with.
	reg := createWorkingRegistration(t, sa)
	serial, testCert := test.ThrowAwayCert(t, fc)
	_, err := sa.AddSerial(ctx, &sapb.AddSerialRequest{
		RegID:   reg.Id,
		Serial:  core.SerialToString(testCert.SerialNumber),
		Created: timestamppb.New(testCert.NotBefore),
		Expires: timestamppb.New(testCert.NotAfter),
	})
	test.AssertNotError(t, err, "failed to add test serial")
	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(testCert.NotBefore),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Couldn't add test cert")
	fc.Add(1 * time.Hour)

	// Now revoke it with a shardIdx, so that it gets updated in both the
	// certificateStatus table and the revokedCertificates table.
	revokedTime := fc.Now()
	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		ShardIdx: 9,
		Serial:   serial,
		Date:     timestamppb.New(revokedTime),
		Reason:   ocsp.CessationOfOperation,
		Response: []byte{1, 2, 3},
	})
	test.AssertNotError(t, err, "RevokeCertificate failed")

	// Updating revocation should succeed, with the revokedCertificates row being
	// updated.
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		ShardIdx: 9,
		Serial:   serial,
		Date:     timestamppb.New(fc.Now()),
		Backdate: timestamppb.New(revokedTime),
		Reason:   ocsp.KeyCompromise,
		Response: []byte{4, 5, 6},
	})
	test.AssertNotError(t, err, "UpdateRevokedCertificate failed")

	var result revokedCertModel
	err = sa.dbMap.SelectOne(
		ctx, &result, `SELECT * FROM revokedCertificates WHERE serial = ?`, serial)
	test.AssertNotError(t, err, "should be exactly one row in revokedCertificates")
	test.AssertEquals(t, result.ShardIdx, int64(9))
	test.AssertEquals(t, result.RevokedReason, revocation.Reason(ocsp.KeyCompromise))
}

func TestUpdateRevokedCertificateWithShardInterim(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires revokedCertificates database table")
	}

	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Add a cert to the DB to test with.
	reg := createWorkingRegistration(t, sa)
	serial, testCert := test.ThrowAwayCert(t, fc)
	_, err := sa.AddSerial(ctx, &sapb.AddSerialRequest{
		RegID:   reg.Id,
		Serial:  serial,
		Created: timestamppb.New(testCert.NotBefore),
		Expires: timestamppb.New(testCert.NotAfter),
	})
	test.AssertNotError(t, err, "failed to add test serial")
	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(testCert.NotBefore),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Couldn't add test cert")
	fc.Add(1 * time.Hour)

	// Now revoke it *without* a shardIdx, so that it only gets updated in the
	// certificateStatus table, and not the revokedCertificates table.
	revokedTime := timestamppb.New(fc.Now())
	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   serial,
		Date:     revokedTime,
		Reason:   ocsp.CessationOfOperation,
		Response: []byte{1, 2, 3},
	})
	test.AssertNotError(t, err, "RevokeCertificate failed")

	// Confirm that setup worked as expected.
	status, err := sa.GetCertificateStatus(
		ctx, &sapb.Serial{Serial: serial})
	test.AssertNotError(t, err, "GetCertificateStatus failed")
	test.AssertEquals(t, core.OCSPStatus(status.Status), core.OCSPStatusRevoked)

	c, err := sa.dbMap.SelectNullInt(
		ctx, "SELECT count(*) FROM revokedCertificates")
	test.AssertNotError(t, err, "SELECT from revokedCertificates failed")
	test.Assert(t, c.Valid, "SELECT from revokedCertificates got no result")
	test.AssertEquals(t, c.Int64, int64(0))

	// Updating revocation should succeed, with a new row being written into the
	// revokedCertificates table.
	_, err = sa.UpdateRevokedCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		ShardIdx: 9,
		Serial:   serial,
		Date:     timestamppb.New(fc.Now()),
		Backdate: revokedTime,
		Reason:   ocsp.KeyCompromise,
		Response: []byte{4, 5, 6},
	})
	test.AssertNotError(t, err, "UpdateRevokedCertificate failed")

	var result revokedCertModel
	err = sa.dbMap.SelectOne(
		ctx, &result, `SELECT * FROM revokedCertificates WHERE serial = ?`, serial)
	test.AssertNotError(t, err, "should be exactly one row in revokedCertificates")
	test.AssertEquals(t, result.ShardIdx, int64(9))
	test.AssertEquals(t, result.RevokedReason, revocation.Reason(ocsp.KeyCompromise))
}

func TestAddCertificateRenewalBit(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	assertIsRenewal := func(t *testing.T, name string, expected bool) {
		t.Helper()
		var count int
		err := sa.dbMap.SelectOne(
			ctx,
			&count,
			`SELECT COUNT(*) FROM issuedNames
		WHERE reversedName = ?
		AND renewal = ?`,
			ReverseName(name),
			expected,
		)
		test.AssertNotError(t, err, "Unexpected error from SelectOne on issuedNames")
		test.AssertEquals(t, count, 1)
	}

	// Add a certificate with a never-before-seen name.
	_, testCert := test.ThrowAwayCert(t, fc)
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		Issued:       timestamppb.New(testCert.NotBefore),
		RegID:        reg.Id,
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Failed to add precertificate")
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(testCert.NotBefore),
	})
	test.AssertNotError(t, err, "Failed to add certificate")

	// None of the names should have a issuedNames row marking it as a renewal.
	for _, name := range testCert.DNSNames {
		assertIsRenewal(t, name, false)
	}

	// Make a new cert and add its FQDN set to the db so it will be considered a
	// renewal
	serial, testCert := test.ThrowAwayCert(t, fc)
	err = addFQDNSet(ctx, sa.dbMap, testCert.DNSNames, serial, testCert.NotBefore, testCert.NotAfter)
	test.AssertNotError(t, err, "Failed to add name set")
	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		Issued:       timestamppb.New(testCert.NotBefore),
		RegID:        reg.Id,
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "Failed to add precertificate")
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  reg.Id,
		Issued: timestamppb.New(testCert.NotBefore),
	})
	test.AssertNotError(t, err, "Failed to add certificate")

	// All of the names should have a issuedNames row marking it as a renewal.
	for _, name := range testCert.DNSNames {
		assertIsRenewal(t, name, true)
	}
}

func TestCountCertificatesRenewalBit(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Create a test registration
	reg := createWorkingRegistration(t, sa)

	// Create a small throw away key for the test certificates.
	testKey, err := rsa.GenerateKey(rand.Reader, 512)
	test.AssertNotError(t, err, "error generating test key")

	// Create an initial test certificate for a set of domain names, issued an
	// hour ago.
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1337),
		DNSNames:              []string{"www.not-example.com", "not-example.com", "admin.not-example.com"},
		NotBefore:             fc.Now().Add(-time.Hour),
		BasicConstraintsValid: true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	certADER, err := x509.CreateCertificate(rand.Reader, template, template, testKey.Public(), testKey)
	test.AssertNotError(t, err, "Failed to create test cert A")
	certA, _ := x509.ParseCertificate(certADER)

	// Update the template with a new serial number and a not before of now and
	// create a second test cert for the same names. This will be a renewal.
	template.SerialNumber = big.NewInt(7331)
	template.NotBefore = fc.Now()
	certBDER, err := x509.CreateCertificate(rand.Reader, template, template, testKey.Public(), testKey)
	test.AssertNotError(t, err, "Failed to create test cert B")
	certB, _ := x509.ParseCertificate(certBDER)

	// Update the template with a third serial number and a partially overlapping
	// set of names. This will not be a renewal but will help test the exact name
	// counts.
	template.SerialNumber = big.NewInt(0xC0FFEE)
	template.DNSNames = []string{"www.not-example.com"}
	certCDER, err := x509.CreateCertificate(rand.Reader, template, template, testKey.Public(), testKey)
	test.AssertNotError(t, err, "Failed to create test cert C")

	countName := func(t *testing.T, expectedName string) int64 {
		earliest := fc.Now().Add(-5 * time.Hour)
		latest := fc.Now().Add(5 * time.Hour)
		req := &sapb.CountCertificatesByNamesRequest{
			Names: []string{expectedName},
			Range: &sapb.Range{
				Earliest: timestamppb.New(earliest),
				Latest:   timestamppb.New(latest),
			},
		}
		counts, err := sa.CountCertificatesByNames(context.Background(), req)
		test.AssertNotError(t, err, "Unexpected err from CountCertificatesByNames")
		for name, count := range counts.Counts {
			if name == expectedName {
				return count
			}
		}
		return 0
	}

	// Add the first certificate - it won't be considered a renewal.
	issued := certA.NotBefore
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    certADER,
		RegID:  reg.Id,
		Issued: timestamppb.New(issued),
	})
	test.AssertNotError(t, err, "Failed to add CertA test certificate")

	// The count for the base domain should be 1 - just certA has been added.
	test.AssertEquals(t, countName(t, "not-example.com"), int64(1))

	// Add the second certificate - it should be considered a renewal
	issued = certB.NotBefore
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    certBDER,
		RegID:  reg.Id,
		Issued: timestamppb.New(issued),
	})
	test.AssertNotError(t, err, "Failed to add CertB test certificate")

	// The count for the base domain should still be 1, just certA. CertB should
	// be ignored.
	test.AssertEquals(t, countName(t, "not-example.com"), int64(1))

	// Add the third certificate - it should not be considered a renewal
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    certCDER,
		RegID:  reg.Id,
		Issued: timestamppb.New(issued),
	})
	test.AssertNotError(t, err, "Failed to add CertC test certificate")

	// The count for the base domain should be 2 now: certA and certC.
	// CertB should be ignored.
	test.AssertEquals(t, countName(t, "not-example.com"), int64(2))
}

func TestFinalizeAuthorization2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	fc.Set(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

	authzID := createPendingAuthorization(t, sa, "aaa", fc.Now().Add(time.Hour))
	expires := fc.Now().Add(time.Hour * 2).UTC()
	attemptedAt := fc.Now()
	ip, _ := net.ParseIP("1.1.1.1").MarshalText()

	_, err := sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id: authzID,
		ValidationRecords: []*corepb.ValidationRecord{
			{
				Hostname:      "example.com",
				Port:          "80",
				Url:           "http://example.com",
				AddressUsed:   ip,
				ResolverAddrs: []string{"resolver:5353"},
			},
		},
		Status:      string(core.StatusValid),
		Expires:     timestamppb.New(expires),
		Attempted:   string(core.ChallengeTypeHTTP01),
		AttemptedAt: timestamppb.New(attemptedAt),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorization2 failed")

	dbVer, err := sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "sa.GetAuthorization2 failed")
	test.AssertEquals(t, dbVer.Status, string(core.StatusValid))
	test.AssertEquals(t, dbVer.Expires.AsTime(), expires)
	test.AssertEquals(t, dbVer.Challenges[0].Status, string(core.StatusValid))
	test.AssertEquals(t, len(dbVer.Challenges[0].Validationrecords), 1)
	test.AssertEquals(t, dbVer.Challenges[0].Validationrecords[0].Hostname, "example.com")
	test.AssertEquals(t, dbVer.Challenges[0].Validationrecords[0].Port, "80")
	test.AssertEquals(t, dbVer.Challenges[0].Validationrecords[0].ResolverAddrs[0], "resolver:5353")
	test.AssertEquals(t, dbVer.Challenges[0].Validated.AsTime(), attemptedAt)

	authzID = createPendingAuthorization(t, sa, "aaa", fc.Now().Add(time.Hour))
	prob, _ := bgrpc.ProblemDetailsToPB(probs.Connection("it went bad captain"))

	_, err = sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id: authzID,
		ValidationRecords: []*corepb.ValidationRecord{
			{
				Hostname:      "example.com",
				Port:          "80",
				Url:           "http://example.com",
				AddressUsed:   ip,
				ResolverAddrs: []string{"resolver:5353"},
			},
		},
		ValidationError: prob,
		Status:          string(core.StatusInvalid),
		Attempted:       string(core.ChallengeTypeHTTP01),
		Expires:         timestamppb.New(expires),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorization2 failed")

	dbVer, err = sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "sa.GetAuthorization2 failed")
	test.AssertEquals(t, dbVer.Status, string(core.StatusInvalid))
	test.AssertEquals(t, dbVer.Challenges[0].Status, string(core.StatusInvalid))
	test.AssertEquals(t, len(dbVer.Challenges[0].Validationrecords), 1)
	test.AssertEquals(t, dbVer.Challenges[0].Validationrecords[0].Hostname, "example.com")
	test.AssertEquals(t, dbVer.Challenges[0].Validationrecords[0].Port, "80")
	test.AssertEquals(t, dbVer.Challenges[0].Validationrecords[0].ResolverAddrs[0], "resolver:5353")
	test.AssertDeepEquals(t, dbVer.Challenges[0].Error, prob)
}

func TestRehydrateHostPort(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	fc.Set(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))

	expires := fc.Now().Add(time.Hour * 2).UTC()
	attemptedAt := fc.Now()
	ip, _ := net.ParseIP("1.1.1.1").MarshalText()

	// Implicit good port with good scheme
	authzID := createPendingAuthorization(t, sa, "aaa", fc.Now().Add(time.Hour))
	_, err := sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id: authzID,
		ValidationRecords: []*corepb.ValidationRecord{
			{
				Hostname:    "example.com",
				Port:        "80",
				Url:         "http://example.com",
				AddressUsed: ip,
			},
		},
		Status:      string(core.StatusValid),
		Expires:     timestamppb.New(expires),
		Attempted:   string(core.ChallengeTypeHTTP01),
		AttemptedAt: timestamppb.New(attemptedAt),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorization2 failed")
	_, err = sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "rehydration failed in some fun and interesting way")

	// Explicit good port with good scheme
	authzID = createPendingAuthorization(t, sa, "aaa", fc.Now().Add(time.Hour))
	_, err = sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id: authzID,
		ValidationRecords: []*corepb.ValidationRecord{
			{
				Hostname:    "example.com",
				Port:        "80",
				Url:         "http://example.com:80",
				AddressUsed: ip,
			},
		},
		Status:      string(core.StatusValid),
		Expires:     timestamppb.New(expires),
		Attempted:   string(core.ChallengeTypeHTTP01),
		AttemptedAt: timestamppb.New(attemptedAt),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorization2 failed")
	_, err = sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "rehydration failed in some fun and interesting way")

	// Explicit bad port with good scheme
	authzID = createPendingAuthorization(t, sa, "aaa", fc.Now().Add(time.Hour))
	_, err = sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id: authzID,
		ValidationRecords: []*corepb.ValidationRecord{
			{
				Hostname:    "example.com",
				Port:        "444",
				Url:         "http://example.com:444",
				AddressUsed: ip,
			},
		},
		Status:      string(core.StatusValid),
		Expires:     timestamppb.New(expires),
		Attempted:   string(core.ChallengeTypeHTTP01),
		AttemptedAt: timestamppb.New(attemptedAt),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorization2 failed")
	_, err = sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertError(t, err, "only ports 80/tcp and 443/tcp are allowed in URL \"http://example.com:444\"")

	// Explicit bad port with bad scheme
	authzID = createPendingAuthorization(t, sa, "aaa", fc.Now().Add(time.Hour))
	_, err = sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id: authzID,
		ValidationRecords: []*corepb.ValidationRecord{
			{
				Hostname:    "example.com",
				Port:        "80",
				Url:         "httpx://example.com",
				AddressUsed: ip,
			},
		},
		Status:      string(core.StatusValid),
		Expires:     timestamppb.New(expires),
		Attempted:   string(core.ChallengeTypeHTTP01),
		AttemptedAt: timestamppb.New(attemptedAt),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorization2 failed")
	_, err = sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertError(t, err, "unknown scheme \"httpx\" in URL \"httpx://example.com\"")

	// Missing URL field
	authzID = createPendingAuthorization(t, sa, "aaa", fc.Now().Add(time.Hour))
	_, err = sa.FinalizeAuthorization2(context.Background(), &sapb.FinalizeAuthorizationRequest{
		Id: authzID,
		ValidationRecords: []*corepb.ValidationRecord{
			{
				Hostname:    "example.com",
				Port:        "80",
				AddressUsed: ip,
			},
		},
		Status:      string(core.StatusValid),
		Expires:     timestamppb.New(expires),
		Attempted:   string(core.ChallengeTypeHTTP01),
		AttemptedAt: timestamppb.New(attemptedAt),
	})
	test.AssertNotError(t, err, "sa.FinalizeAuthorization2 failed")
	_, err = sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertError(t, err, "URL field cannot be empty")
}

func TestGetPendingAuthorization2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	domain := "example.com"
	expiresA := fc.Now().Add(time.Hour).UTC()
	expiresB := fc.Now().Add(time.Hour * 3).UTC()
	authzIDA := createPendingAuthorization(t, sa, domain, expiresA)
	authzIDB := createPendingAuthorization(t, sa, domain, expiresB)

	regID := int64(1)
	validUntil := fc.Now().Add(time.Hour * 2).UTC()
	dbVer, err := sa.GetPendingAuthorization2(context.Background(), &sapb.GetPendingAuthorizationRequest{
		RegistrationID:  regID,
		IdentifierValue: domain,
		ValidUntil:      timestamppb.New(validUntil),
	})
	test.AssertNotError(t, err, "sa.GetPendingAuthorization2 failed")
	test.AssertEquals(t, fmt.Sprintf("%d", authzIDB), dbVer.Id)

	validUntil = fc.Now().UTC()
	dbVer, err = sa.GetPendingAuthorization2(context.Background(), &sapb.GetPendingAuthorizationRequest{
		RegistrationID:  regID,
		IdentifierValue: domain,
		ValidUntil:      timestamppb.New(validUntil),
	})
	test.AssertNotError(t, err, "sa.GetPendingAuthorization2 failed")
	test.AssertEquals(t, fmt.Sprintf("%d", authzIDA), dbVer.Id)
}

func TestCountPendingAuthorizations2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	expiresA := fc.Now().Add(time.Hour).UTC()
	expiresB := fc.Now().Add(time.Hour * 3).UTC()
	_ = createPendingAuthorization(t, sa, "example.com", expiresA)
	_ = createPendingAuthorization(t, sa, "example.com", expiresB)

	// Registration has two new style pending authorizations
	regID := int64(1)
	count, err := sa.CountPendingAuthorizations2(context.Background(), &sapb.RegistrationID{
		Id: regID,
	})
	test.AssertNotError(t, err, "sa.CountPendingAuthorizations2 failed")
	test.AssertEquals(t, count.Count, int64(2))

	// Registration has two new style pending authorizations, one of which has expired
	fc.Add(time.Hour * 2)
	count, err = sa.CountPendingAuthorizations2(context.Background(), &sapb.RegistrationID{
		Id: regID,
	})
	test.AssertNotError(t, err, "sa.CountPendingAuthorizations2 failed")
	test.AssertEquals(t, count.Count, int64(1))

	// Registration with no authorizations should be 0
	noReg := int64(20)
	count, err = sa.CountPendingAuthorizations2(context.Background(), &sapb.RegistrationID{
		Id: noReg,
	})
	test.AssertNotError(t, err, "sa.CountPendingAuthorizations2 failed")
	test.AssertEquals(t, count.Count, int64(0))
}

func TestAuthzModelMapToPB(t *testing.T) {
	baseExpires := time.Now()
	input := map[string]authzModel{
		"example.com": {
			ID:              123,
			IdentifierType:  0,
			IdentifierValue: "example.com",
			RegistrationID:  77,
			Status:          1,
			Expires:         baseExpires,
			Challenges:      4,
		},
		"www.example.com": {
			ID:              124,
			IdentifierType:  0,
			IdentifierValue: "www.example.com",
			RegistrationID:  77,
			Status:          1,
			Expires:         baseExpires,
			Challenges:      1,
		},
		"other.example.net": {
			ID:              125,
			IdentifierType:  0,
			IdentifierValue: "other.example.net",
			RegistrationID:  77,
			Status:          1,
			Expires:         baseExpires,
			Challenges:      3,
		},
	}

	out, err := authzModelMapToPB(input)
	if err != nil {
		t.Fatal(err)
	}

	for _, el := range out.Authz {
		model, ok := input[el.Domain]
		if !ok {
			t.Errorf("output had element for %q, a hostname not present in input", el.Domain)
		}
		authzPB := el.Authz
		test.AssertEquals(t, authzPB.Id, fmt.Sprintf("%d", model.ID))
		test.AssertEquals(t, authzPB.Identifier, model.IdentifierValue)
		test.AssertEquals(t, authzPB.RegistrationID, model.RegistrationID)
		test.AssertEquals(t, authzPB.Status, string(uintToStatus[model.Status]))
		gotTime := authzPB.Expires.AsTime()
		if !model.Expires.Equal(gotTime) {
			t.Errorf("Times didn't match. Got %s, expected %s (%s)", gotTime, model.Expires, authzPB.Expires.AsTime())
		}
		if len(el.Authz.Challenges) != bits.OnesCount(uint(model.Challenges)) {
			t.Errorf("wrong number of challenges for %q: got %d, expected %d", el.Domain,
				len(el.Authz.Challenges), bits.OnesCount(uint(model.Challenges)))
		}
		switch model.Challenges {
		case 1:
			test.AssertEquals(t, el.Authz.Challenges[0].Type, "http-01")
		case 3:
			test.AssertEquals(t, el.Authz.Challenges[0].Type, "http-01")
			test.AssertEquals(t, el.Authz.Challenges[1].Type, "dns-01")
		case 4:
			test.AssertEquals(t, el.Authz.Challenges[0].Type, "tls-alpn-01")
		}

		delete(input, el.Domain)
	}

	for k := range input {
		t.Errorf("hostname %q was not present in output", k)
	}
}

func TestGetValidOrderAuthorizations2(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	// Create two new valid authorizations
	reg := createWorkingRegistration(t, sa)
	identA := "a.example.com"
	identB := "b.example.com"
	expires := fc.Now().Add(time.Hour * 24 * 7).UTC()
	attemptedAt := fc.Now()

	authzIDA := createFinalizedAuthorization(t, sa, identA, expires, "valid", attemptedAt)
	authzIDB := createFinalizedAuthorization(t, sa, identB, expires, "valid", attemptedAt)

	orderExpr := fc.Now().Truncate(time.Second)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(orderExpr),
			Names:            []string{"a.example.com", "b.example.com"},
			V2Authorizations: []int64{authzIDA, authzIDB},
		},
	})
	test.AssertNotError(t, err, "AddOrder failed")

	authzMap, err := sa.GetValidOrderAuthorizations2(
		context.Background(),
		&sapb.GetValidOrderAuthorizationsRequest{
			Id:     order.Id,
			AcctID: reg.Id,
		})
	test.AssertNotError(t, err, "sa.GetValidOrderAuthorizations failed")
	test.AssertNotNil(t, authzMap, "sa.GetValidOrderAuthorizations result was nil")
	test.AssertEquals(t, len(authzMap.Authz), 2)

	namesToCheck := map[string]int64{"a.example.com": authzIDA, "b.example.com": authzIDB}
	for _, a := range authzMap.Authz {
		if fmt.Sprintf("%d", namesToCheck[a.Authz.Identifier]) != a.Authz.Id {
			t.Fatalf("incorrect identifier %q with id %s", a.Authz.Identifier, a.Authz.Id)
		}
		test.AssertEquals(t, a.Authz.Expires.AsTime(), expires)
		delete(namesToCheck, a.Authz.Identifier)
	}

	// Getting the order authorizations for an order that doesn't exist should return nothing
	missingID := int64(0xC0FFEEEEEEE)
	authzMap, err = sa.GetValidOrderAuthorizations2(
		context.Background(),
		&sapb.GetValidOrderAuthorizationsRequest{
			Id:     missingID,
			AcctID: reg.Id,
		})
	test.AssertNotError(t, err, "sa.GetValidOrderAuthorizations failed")
	test.AssertEquals(t, len(authzMap.Authz), 0)

	// Getting the order authorizations for an order that does exist, but for the
	// wrong acct ID should return nothing
	wrongAcctID := int64(0xDEADDA7ABA5E)
	authzMap, err = sa.GetValidOrderAuthorizations2(
		context.Background(),
		&sapb.GetValidOrderAuthorizationsRequest{
			Id:     order.Id,
			AcctID: wrongAcctID,
		})
	test.AssertNotError(t, err, "sa.GetValidOrderAuthorizations failed")
	test.AssertEquals(t, len(authzMap.Authz), 0)
}

func TestCountInvalidAuthorizations2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Create two authorizations, one pending, one invalid
	fc.Add(time.Hour)
	reg := createWorkingRegistration(t, sa)
	ident := "aaa"
	expiresA := fc.Now().Add(time.Hour).UTC()
	expiresB := fc.Now().Add(time.Hour * 3).UTC()
	attemptedAt := fc.Now()
	_ = createFinalizedAuthorization(t, sa, ident, expiresA, "invalid", attemptedAt)
	_ = createPendingAuthorization(t, sa, ident, expiresB)

	earliest := fc.Now().Add(-time.Hour).UTC()
	latest := fc.Now().Add(time.Hour * 5).UTC()
	count, err := sa.CountInvalidAuthorizations2(context.Background(), &sapb.CountInvalidAuthorizationsRequest{
		RegistrationID: reg.Id,
		Hostname:       ident,
		Range: &sapb.Range{
			Earliest: timestamppb.New(earliest),
			Latest:   timestamppb.New(latest),
		},
	})
	test.AssertNotError(t, err, "sa.CountInvalidAuthorizations2 failed")
	test.AssertEquals(t, count.Count, int64(1))
}

func TestGetValidAuthorizations2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Create a valid authorization
	ident := "aaa"
	expires := fc.Now().Add(time.Hour).UTC()
	attemptedAt := fc.Now()
	authzID := createFinalizedAuthorization(t, sa, ident, expires, "valid", attemptedAt)

	now := fc.Now().UTC()
	regID := int64(1)
	authzs, err := sa.GetValidAuthorizations2(context.Background(), &sapb.GetValidAuthorizationsRequest{
		Domains: []string{
			"aaa",
			"bbb",
		},
		RegistrationID: regID,
		Now:            timestamppb.New(now),
	})
	test.AssertNotError(t, err, "sa.GetValidAuthorizations2 failed")
	test.AssertEquals(t, len(authzs.Authz), 1)
	test.AssertEquals(t, authzs.Authz[0].Domain, ident)
	test.AssertEquals(t, authzs.Authz[0].Authz.Id, fmt.Sprintf("%d", authzID))
}

func TestGetOrderExpired(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()
	fc.Add(time.Hour * 5)
	now := fc.Now()
	reg := createWorkingRegistration(t, sa)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(now.Add(-time.Hour)),
			Names:            []string{"example.com"},
			V2Authorizations: []int64{666},
		},
	})
	test.AssertNotError(t, err, "NewOrderAndAuthzs failed")
	_, err = sa.GetOrder(context.Background(), &sapb.OrderRequest{
		Id: order.Id,
	})
	test.AssertError(t, err, "GetOrder didn't fail for an expired order")
	test.AssertErrorIs(t, err, berrors.NotFound)
}

func TestBlockedKey(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	hashA := make([]byte, 32)
	hashA[0] = 1
	hashB := make([]byte, 32)
	hashB[0] = 2

	added := time.Now()
	source := "API"
	_, err := sa.AddBlockedKey(context.Background(), &sapb.AddBlockedKeyRequest{
		KeyHash: hashA,
		Added:   timestamppb.New(added),
		Source:  source,
	})
	test.AssertNotError(t, err, "AddBlockedKey failed")
	_, err = sa.AddBlockedKey(context.Background(), &sapb.AddBlockedKeyRequest{
		KeyHash: hashA,
		Added:   timestamppb.New(added),
		Source:  source,
	})
	test.AssertNotError(t, err, "AddBlockedKey failed with duplicate insert")

	comment := "testing comments"
	_, err = sa.AddBlockedKey(context.Background(), &sapb.AddBlockedKeyRequest{
		KeyHash: hashB,
		Added:   timestamppb.New(added),
		Source:  source,
		Comment: comment,
	})
	test.AssertNotError(t, err, "AddBlockedKey failed")

	exists, err := sa.KeyBlocked(context.Background(), &sapb.SPKIHash{
		KeyHash: hashA,
	})
	test.AssertNotError(t, err, "KeyBlocked failed")
	test.Assert(t, exists != nil, "*sapb.Exists is nil")
	test.Assert(t, exists.Exists, "KeyBlocked returned false for blocked key")
	exists, err = sa.KeyBlocked(context.Background(), &sapb.SPKIHash{
		KeyHash: hashB,
	})
	test.AssertNotError(t, err, "KeyBlocked failed")
	test.Assert(t, exists != nil, "*sapb.Exists is nil")
	test.Assert(t, exists.Exists, "KeyBlocked returned false for blocked key")
	exists, err = sa.KeyBlocked(context.Background(), &sapb.SPKIHash{
		KeyHash: []byte{5},
	})
	test.AssertNotError(t, err, "KeyBlocked failed")
	test.Assert(t, exists != nil, "*sapb.Exists is nil")
	test.Assert(t, !exists.Exists, "KeyBlocked returned true for non-blocked key")
}

func TestAddBlockedKeyUnknownSource(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	_, err := sa.AddBlockedKey(context.Background(), &sapb.AddBlockedKeyRequest{
		KeyHash: []byte{1, 2, 3},
		Added:   timestamppb.New(fc.Now()),
		Source:  "heyo",
	})
	test.AssertError(t, err, "AddBlockedKey didn't fail with unknown source")
	test.AssertEquals(t, err.Error(), "unknown source")
}

func TestBlockedKeyRevokedBy(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	now := fc.Now()
	_, err := sa.AddBlockedKey(context.Background(), &sapb.AddBlockedKeyRequest{
		KeyHash: []byte{1},
		Added:   timestamppb.New(now),
		Source:  "API",
	})
	test.AssertNotError(t, err, "AddBlockedKey failed")

	_, err = sa.AddBlockedKey(context.Background(), &sapb.AddBlockedKeyRequest{
		KeyHash:   []byte{2},
		Added:     timestamppb.New(now),
		Source:    "API",
		RevokedBy: 1,
	})
	test.AssertNotError(t, err, "AddBlockedKey failed")
}

func TestIncidentsForSerial(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	testSADbMap, err := DBMapForTest(vars.DBConnSAFullPerms)
	test.AssertNotError(t, err, "Couldn't create test dbMap")

	testIncidentsDbMap, err := DBMapForTest(vars.DBConnIncidentsFullPerms)
	test.AssertNotError(t, err, "Couldn't create test dbMap")
	defer test.ResetIncidentsTestDatabase(t)

	weekAgo := sa.clk.Now().Add(-time.Hour * 24 * 7)

	// Add a disabled incident.
	err = testSADbMap.Insert(ctx, &incidentModel{
		SerialTable: "incident_foo",
		URL:         "https://example.com/foo-incident",
		RenewBy:     sa.clk.Now().Add(time.Hour * 24 * 7),
		Enabled:     false,
	})
	test.AssertNotError(t, err, "Failed to insert disabled incident")

	// No incidents are enabled, so this should return in error.
	result, err := sa.IncidentsForSerial(context.Background(), &sapb.Serial{Serial: "1337"})
	test.AssertNotError(t, err, "fetching from no incidents")
	test.AssertEquals(t, len(result.Incidents), 0)

	// Add an enabled incident.
	err = testSADbMap.Insert(ctx, &incidentModel{
		SerialTable: "incident_bar",
		URL:         "https://example.com/test-incident",
		RenewBy:     sa.clk.Now().Add(time.Hour * 24 * 7),
		Enabled:     true,
	})
	test.AssertNotError(t, err, "Failed to insert enabled incident")

	// Add a row to the incident table with serial '1338'.
	one := int64(1)
	affectedCertA := incidentSerialModel{
		Serial:         "1338",
		RegistrationID: &one,
		OrderID:        &one,
		LastNoticeSent: &weekAgo,
	}
	_, err = testIncidentsDbMap.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO incident_bar (%s) VALUES ('%s', %d, %d, '%s')",
			"serial, registrationID, orderID, lastNoticeSent",
			affectedCertA.Serial,
			affectedCertA.RegistrationID,
			affectedCertA.OrderID,
			affectedCertA.LastNoticeSent.Format(time.DateTime),
		),
	)
	test.AssertNotError(t, err, "Error while inserting row for '1338' into incident table")

	// The incident table should not contain a row with serial '1337'.
	result, err = sa.IncidentsForSerial(context.Background(), &sapb.Serial{Serial: "1337"})
	test.AssertNotError(t, err, "fetching from one incident")
	test.AssertEquals(t, len(result.Incidents), 0)

	// Add a row to the incident table with serial '1337'.
	two := int64(2)
	affectedCertB := incidentSerialModel{
		Serial:         "1337",
		RegistrationID: &two,
		OrderID:        &two,
		LastNoticeSent: &weekAgo,
	}
	_, err = testIncidentsDbMap.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO incident_bar (%s) VALUES ('%s', %d, %d, '%s')",
			"serial, registrationID, orderID, lastNoticeSent",
			affectedCertB.Serial,
			affectedCertB.RegistrationID,
			affectedCertB.OrderID,
			affectedCertB.LastNoticeSent.Format(time.DateTime),
		),
	)
	test.AssertNotError(t, err, "Error while inserting row for '1337' into incident table")

	// The incident table should now contain a row with serial '1337'.
	result, err = sa.IncidentsForSerial(context.Background(), &sapb.Serial{Serial: "1337"})
	test.AssertNotError(t, err, "Failed to retrieve incidents for serial")
	test.AssertEquals(t, len(result.Incidents), 1)
}

func TestSerialsForIncident(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	testIncidentsDbMap, err := DBMapForTest(vars.DBConnIncidentsFullPerms)
	test.AssertNotError(t, err, "Couldn't create test dbMap")
	defer test.ResetIncidentsTestDatabase(t)

	// Request serials from a malformed incident table name.
	mockServerStream := &fakeServerStream[sapb.IncidentSerial]{}
	err = sa.SerialsForIncident(
		&sapb.SerialsForIncidentRequest{
			IncidentTable: "incidesnt_Baz",
		},
		mockServerStream,
	)
	test.AssertError(t, err, "Expected error for malformed table name")
	test.AssertContains(t, err.Error(), "malformed table name \"incidesnt_Baz\"")

	// Request serials from another malformed incident table name.
	mockServerStream = &fakeServerStream[sapb.IncidentSerial]{}
	longTableName := "incident_l" + strings.Repeat("o", 1000) + "ng"
	err = sa.SerialsForIncident(
		&sapb.SerialsForIncidentRequest{
			IncidentTable: longTableName,
		},
		mockServerStream,
	)
	test.AssertError(t, err, "Expected error for long table name")
	test.AssertContains(t, err.Error(), fmt.Sprintf("malformed table name %q", longTableName))

	// Request serials for an incident table which doesn't exists.
	mockServerStream = &fakeServerStream[sapb.IncidentSerial]{}
	err = sa.SerialsForIncident(
		&sapb.SerialsForIncidentRequest{
			IncidentTable: "incident_baz",
		},
		mockServerStream,
	)
	test.AssertError(t, err, "Expected error for nonexistent table name")

	// Assert that the error is a MySQL error so we can inspect the error code.
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		// We expect the error code to be 1146 (ER_NO_SUCH_TABLE):
		// https://mariadb.com/kb/en/mariadb-error-codes/
		test.AssertEquals(t, mysqlErr.Number, uint16(1146))
	} else {
		t.Fatalf("Expected MySQL Error 1146 (ER_NO_SUCH_TABLE) from Recv(), got %q", err)
	}

	// Request serials from table 'incident_foo', which we expect to exist but
	// be empty.
	stream := make(chan *sapb.IncidentSerial)
	mockServerStream = &fakeServerStream[sapb.IncidentSerial]{output: stream}
	go func() {
		err = sa.SerialsForIncident(
			&sapb.SerialsForIncidentRequest{
				IncidentTable: "incident_foo",
			},
			mockServerStream,
		)
		close(stream) // Let our main test thread continue.
	}()
	for range stream {
		t.Fatal("No serials should have been written to this stream")
	}
	test.AssertNotError(t, err, "Error calling SerialsForIncident on empty table")

	// Add 4 rows of incident serials to 'incident_foo'.
	expectedSerials := map[string]bool{
		"1335": true, "1336": true, "1337": true, "1338": true,
	}
	for i := range expectedSerials {
		randInt := func() int64 { return mrand.Int63() }
		_, err := testIncidentsDbMap.ExecContext(ctx,
			fmt.Sprintf("INSERT INTO incident_foo (%s) VALUES ('%s', %d, %d, '%s')",
				"serial, registrationID, orderID, lastNoticeSent",
				i,
				randInt(),
				randInt(),
				sa.clk.Now().Add(time.Hour*24*7).Format(time.DateTime),
			),
		)
		test.AssertNotError(t, err, fmt.Sprintf("Error while inserting row for '%s' into incident table", i))
	}

	// Request all 4 serials from the incident table we just added entries to.
	stream = make(chan *sapb.IncidentSerial)
	mockServerStream = &fakeServerStream[sapb.IncidentSerial]{output: stream}
	go func() {
		err = sa.SerialsForIncident(
			&sapb.SerialsForIncidentRequest{
				IncidentTable: "incident_foo",
			},
			mockServerStream,
		)
		close(stream)
	}()
	receivedSerials := make(map[string]bool)
	for serial := range stream {
		if len(receivedSerials) > 4 {
			t.Fatal("Received too many serials")
		}
		if _, ok := receivedSerials[serial.Serial]; ok {
			t.Fatalf("Received serial %q more than once", serial.Serial)
		}
		receivedSerials[serial.Serial] = true
	}
	test.AssertDeepEquals(t, receivedSerials, map[string]bool{
		"1335": true, "1336": true, "1337": true, "1338": true,
	})
	test.AssertNotError(t, err, "Error getting serials for incident")
}

func TestGetRevokedCerts(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	// Add a cert to the DB to test with. We use AddPrecertificate because it sets
	// up the certificateStatus row we need. This particular cert has a notAfter
	// date of Mar 6 2023, and we lie about its IssuerNameID to make things easy.
	reg := createWorkingRegistration(t, sa)
	eeCert, err := core.LoadCert("../test/hierarchy/ee-e1.cert.pem")
	test.AssertNotError(t, err, "failed to load test cert")
	_, err = sa.AddSerial(ctx, &sapb.AddSerialRequest{
		RegID:   reg.Id,
		Serial:  core.SerialToString(eeCert.SerialNumber),
		Created: timestamppb.New(eeCert.NotBefore),
		Expires: timestamppb.New(eeCert.NotAfter),
	})
	test.AssertNotError(t, err, "failed to add test serial")
	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          eeCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(eeCert.NotBefore),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "failed to add test cert")

	// Check that it worked.
	status, err := sa.GetCertificateStatus(
		ctx, &sapb.Serial{Serial: core.SerialToString(eeCert.SerialNumber)})
	test.AssertNotError(t, err, "GetCertificateStatus failed")
	test.AssertEquals(t, core.OCSPStatus(status.Status), core.OCSPStatusGood)

	// Here's a little helper func we'll use to call GetRevokedCerts and count
	// how many results it returned.
	countRevokedCerts := func(req *sapb.GetRevokedCertsRequest) (int, error) {
		stream := make(chan *corepb.CRLEntry)
		mockServerStream := &fakeServerStream[corepb.CRLEntry]{output: stream}
		var err error
		go func() {
			err = sa.GetRevokedCerts(req, mockServerStream)
			close(stream)
		}()
		entriesReceived := 0
		for range stream {
			entriesReceived++
		}
		return entriesReceived, err
	}

	// Asking for revoked certs now should return no results.
	expiresAfter := time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC)
	expiresBefore := time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore := time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	count, err := countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		ExpiresBefore: timestamppb.New(expiresBefore),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Revoke the certificate.
	date := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   core.SerialToString(eeCert.SerialNumber),
		Date:     timestamppb.New(date),
		Reason:   1,
		Response: []byte{1, 2, 3},
	})
	test.AssertNotError(t, err, "failed to revoke test cert")

	// Asking for revoked certs now should return one result.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		ExpiresBefore: timestamppb.New(expiresBefore),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "normal usage shouldn't result in error")
	test.AssertEquals(t, count, 1)

	// Asking for revoked certs with an old RevokedBefore should return no results.
	expiresAfter = time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC)
	expiresBefore = time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore = time.Date(2020, time.March, 1, 0, 0, 0, 0, time.UTC)
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		ExpiresBefore: timestamppb.New(expiresBefore),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs in a time period that does not cover this cert's
	// notAfter timestamp should return zero results.
	expiresAfter = time.Date(2022, time.March, 1, 0, 0, 0, 0, time.UTC)
	expiresBefore = time.Date(2022, time.April, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore = time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		ExpiresBefore: timestamppb.New(expiresBefore),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs from a different issuer should return zero results.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  timestamppb.New(time.Date(2022, time.March, 1, 0, 0, 0, 0, time.UTC)),
		ExpiresBefore: timestamppb.New(time.Date(2022, time.April, 1, 0, 0, 0, 0, time.UTC)),
		RevokedBefore: timestamppb.New(time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)
}

func TestGetRevokedCertsByShard(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires revokedCertificates database table")
	}

	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	// Add a cert to the DB to test with. We use AddPrecertificate because it sets
	// up the certificateStatus row we need. This particular cert has a notAfter
	// date of Mar 6 2023, and we lie about its IssuerNameID to make things easy.
	reg := createWorkingRegistration(t, sa)
	eeCert, err := core.LoadCert("../test/hierarchy/ee-e1.cert.pem")
	test.AssertNotError(t, err, "failed to load test cert")
	_, err = sa.AddSerial(ctx, &sapb.AddSerialRequest{
		RegID:   reg.Id,
		Serial:  core.SerialToString(eeCert.SerialNumber),
		Created: timestamppb.New(eeCert.NotBefore),
		Expires: timestamppb.New(eeCert.NotAfter),
	})
	test.AssertNotError(t, err, "failed to add test serial")
	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          eeCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(eeCert.NotBefore),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "failed to add test cert")

	// Check that it worked.
	status, err := sa.GetCertificateStatus(
		ctx, &sapb.Serial{Serial: core.SerialToString(eeCert.SerialNumber)})
	test.AssertNotError(t, err, "GetCertificateStatus failed")
	test.AssertEquals(t, core.OCSPStatus(status.Status), core.OCSPStatusGood)

	// Here's a little helper func we'll use to call GetRevokedCerts and count
	// how many results it returned.
	countRevokedCerts := func(req *sapb.GetRevokedCertsRequest) (int, error) {
		stream := make(chan *corepb.CRLEntry)
		mockServerStream := &fakeServerStream[corepb.CRLEntry]{output: stream}
		var err error
		go func() {
			err = sa.GetRevokedCerts(req, mockServerStream)
			close(stream)
		}()
		entriesReceived := 0
		for range stream {
			entriesReceived++
		}
		return entriesReceived, err
	}

	// Asking for revoked certs now should return no results.
	expiresAfter := time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore := time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	count, err := countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ShardIdx:      9,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Revoke the certificate, providing the ShardIdx so it gets written into
	// both the certificateStatus and revokedCertificates tables.
	date := time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   core.SerialToString(eeCert.SerialNumber),
		Date:     timestamppb.New(date),
		Reason:   1,
		Response: []byte{1, 2, 3},
		ShardIdx: 9,
	})
	test.AssertNotError(t, err, "failed to revoke test cert")

	// Check that it worked in the most basic way.
	c, err := sa.dbMap.SelectNullInt(
		ctx, "SELECT count(*) FROM revokedCertificates")
	test.AssertNotError(t, err, "SELECT from revokedCertificates failed")
	test.Assert(t, c.Valid, "SELECT from revokedCertificates got no result")
	test.AssertEquals(t, c.Int64, int64(1))

	// Asking for revoked certs now should return one result.
	expiresAfter = time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore = time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ShardIdx:      9,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "normal usage shouldn't result in error")
	test.AssertEquals(t, count, 1)

	// Asking for revoked certs from a different issuer should return zero results.
	expiresAfter = time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore = time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  2,
		ShardIdx:      9,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs from a different shard should return zero results.
	expiresAfter = time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore = time.Date(2023, time.April, 1, 0, 0, 0, 0, time.UTC)
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ShardIdx:      8,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs with an old RevokedBefore should return no results.
	expiresAfter = time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC)
	revokedBefore = time.Date(2020, time.March, 1, 0, 0, 0, 0, time.UTC)
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ShardIdx:      9,
		ExpiresAfter:  timestamppb.New(expiresAfter),
		RevokedBefore: timestamppb.New(revokedBefore),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)
}

func TestGetMaxExpiration(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	// Add a cert to the DB to test with. We use AddPrecertificate because it sets
	// up the certificateStatus row we need. This particular cert has a notAfter
	// date of Mar 6 2023, and we lie about its IssuerNameID to make things easy.
	reg := createWorkingRegistration(t, sa)
	eeCert, err := core.LoadCert("../test/hierarchy/ee-e1.cert.pem")
	test.AssertNotError(t, err, "failed to load test cert")
	_, err = sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          eeCert.Raw,
		RegID:        reg.Id,
		Issued:       timestamppb.New(eeCert.NotBefore),
		IssuerNameID: 1,
	})
	test.AssertNotError(t, err, "failed to add test cert")

	lastExpiry, err := sa.GetMaxExpiration(context.Background(), &emptypb.Empty{})
	test.AssertNotError(t, err, "getting last expriy should succeed")
	test.Assert(t, lastExpiry.AsTime().Equal(eeCert.NotAfter), "times should be equal")
	test.AssertEquals(t, timestamppb.New(eeCert.NotBefore).AsTime(), eeCert.NotBefore)
}

func TestLeaseOldestCRLShard(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	// Create 8 shards: 4 for each of 2 issuers. For each issuer, one shard is
	// currently leased, three are available, and one of those failed to update.
	_, err := sa.dbMap.ExecContext(ctx,
		`INSERT INTO crlShards (issuerID, idx, thisUpdate, nextUpdate, leasedUntil) VALUES
		(1, 0, ?, ?, ?),
		(1, 1, ?, ?, ?),
		(1, 2, ?, ?, ?),
		(1, 3, NULL, NULL, ?),
		(2, 0, ?, ?, ?),
		(2, 1, ?, ?, ?),
		(2, 2, ?, ?, ?),
		(2, 3, NULL, NULL, ?);`,
		clk.Now().Add(-7*24*time.Hour), clk.Now().Add(3*24*time.Hour), clk.Now().Add(time.Hour),
		clk.Now().Add(-6*24*time.Hour), clk.Now().Add(4*24*time.Hour), clk.Now().Add(-6*24*time.Hour),
		clk.Now().Add(-5*24*time.Hour), clk.Now().Add(5*24*time.Hour), clk.Now().Add(-5*24*time.Hour),
		clk.Now().Add(-4*24*time.Hour),
		clk.Now().Add(-7*24*time.Hour), clk.Now().Add(3*24*time.Hour), clk.Now().Add(time.Hour),
		clk.Now().Add(-6*24*time.Hour), clk.Now().Add(4*24*time.Hour), clk.Now().Add(-6*24*time.Hour),
		clk.Now().Add(-5*24*time.Hour), clk.Now().Add(5*24*time.Hour), clk.Now().Add(-5*24*time.Hour),
		clk.Now().Add(-4*24*time.Hour),
	)
	test.AssertNotError(t, err, "setting up test shards")

	until := clk.Now().Add(time.Hour).Truncate(time.Second).UTC()
	var untilModel struct {
		LeasedUntil time.Time `db:"leasedUntil"`
	}

	// Leasing from a fully-leased subset should fail.
	_, err = sa.leaseOldestCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 1,
			MinShardIdx:  0,
			MaxShardIdx:  0,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertError(t, err, "leasing when all shards are leased")

	// Leasing any known shard should return the never-before-leased one (3).
	res, err := sa.leaseOldestCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 1,
			MinShardIdx:  0,
			MaxShardIdx:  3,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertNotError(t, err, "leasing available shard")
	test.AssertEquals(t, res.IssuerNameID, int64(1))
	test.AssertEquals(t, res.ShardIdx, int64(3))

	err = sa.dbMap.SelectOne(
		ctx,
		&untilModel,
		`SELECT leasedUntil FROM crlShards WHERE issuerID = ? AND idx = ? LIMIT 1`,
		res.IssuerNameID,
		res.ShardIdx,
	)
	test.AssertNotError(t, err, "getting updated lease timestamp")
	test.Assert(t, untilModel.LeasedUntil.Equal(until), "checking updated lease timestamp")

	// Leasing any known shard *again* should now return the oldest one (1).
	res, err = sa.leaseOldestCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 1,
			MinShardIdx:  0,
			MaxShardIdx:  3,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertNotError(t, err, "leasing available shard")
	test.AssertEquals(t, res.IssuerNameID, int64(1))
	test.AssertEquals(t, res.ShardIdx, int64(1))

	err = sa.dbMap.SelectOne(
		ctx,
		&untilModel,
		`SELECT leasedUntil FROM crlShards WHERE issuerID = ? AND idx = ? LIMIT 1`,
		res.IssuerNameID,
		res.ShardIdx,
	)
	test.AssertNotError(t, err, "getting updated lease timestamp")
	test.Assert(t, untilModel.LeasedUntil.Equal(until), "checking updated lease timestamp")

	// Leasing from a superset of known shards should succeed and return one of
	// the previously-unknown shards.
	res, err = sa.leaseOldestCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 2,
			MinShardIdx:  0,
			MaxShardIdx:  7,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertNotError(t, err, "leasing available shard")
	test.AssertEquals(t, res.IssuerNameID, int64(2))
	test.Assert(t, res.ShardIdx >= 4, "checking leased index")
	test.Assert(t, res.ShardIdx <= 7, "checking leased index")

	err = sa.dbMap.SelectOne(
		ctx,
		&untilModel,
		`SELECT leasedUntil FROM crlShards WHERE issuerID = ? AND idx = ? LIMIT 1`,
		res.IssuerNameID,
		res.ShardIdx,
	)
	test.AssertNotError(t, err, "getting updated lease timestamp")
	test.Assert(t, untilModel.LeasedUntil.Equal(until), "checking updated lease timestamp")
}

func TestLeaseSpecificCRLShard(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	// Create 8 shards: 4 for each of 2 issuers. For each issuer, one shard is
	// currently leased, three are available, and one of those failed to update.
	_, err := sa.dbMap.ExecContext(ctx,
		`INSERT INTO crlShards (issuerID, idx, thisUpdate, nextUpdate, leasedUntil) VALUES
		(1, 0, ?, ?, ?),
		(1, 1, ?, ?, ?),
		(1, 2, ?, ?, ?),
		(1, 3, NULL, NULL, ?),
		(2, 0, ?, ?, ?),
		(2, 1, ?, ?, ?),
		(2, 2, ?, ?, ?),
		(2, 3, NULL, NULL, ?);`,
		clk.Now().Add(-7*24*time.Hour), clk.Now().Add(3*24*time.Hour), clk.Now().Add(time.Hour),
		clk.Now().Add(-6*24*time.Hour), clk.Now().Add(4*24*time.Hour), clk.Now().Add(-6*24*time.Hour),
		clk.Now().Add(-5*24*time.Hour), clk.Now().Add(5*24*time.Hour), clk.Now().Add(-5*24*time.Hour),
		clk.Now().Add(-4*24*time.Hour),
		clk.Now().Add(-7*24*time.Hour), clk.Now().Add(3*24*time.Hour), clk.Now().Add(time.Hour),
		clk.Now().Add(-6*24*time.Hour), clk.Now().Add(4*24*time.Hour), clk.Now().Add(-6*24*time.Hour),
		clk.Now().Add(-5*24*time.Hour), clk.Now().Add(5*24*time.Hour), clk.Now().Add(-5*24*time.Hour),
		clk.Now().Add(-4*24*time.Hour),
	)
	test.AssertNotError(t, err, "setting up test shards")

	until := clk.Now().Add(time.Hour).Truncate(time.Second).UTC()
	var untilModel struct {
		LeasedUntil time.Time `db:"leasedUntil"`
	}

	// Leasing an unleased shard should work.
	res, err := sa.leaseSpecificCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 1,
			MinShardIdx:  1,
			MaxShardIdx:  1,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertNotError(t, err, "leasing available shard")
	test.AssertEquals(t, res.IssuerNameID, int64(1))
	test.AssertEquals(t, res.ShardIdx, int64(1))

	err = sa.dbMap.SelectOne(
		ctx,
		&untilModel,
		`SELECT leasedUntil FROM crlShards WHERE issuerID = ? AND idx = ? LIMIT 1`,
		res.IssuerNameID,
		res.ShardIdx,
	)
	test.AssertNotError(t, err, "getting updated lease timestamp")
	test.Assert(t, untilModel.LeasedUntil.Equal(until), "checking updated lease timestamp")

	// Leasing a never-before-leased shard should work.
	res, err = sa.leaseSpecificCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 2,
			MinShardIdx:  3,
			MaxShardIdx:  3,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertNotError(t, err, "leasing available shard")
	test.AssertEquals(t, res.IssuerNameID, int64(2))
	test.AssertEquals(t, res.ShardIdx, int64(3))

	err = sa.dbMap.SelectOne(
		ctx,
		&untilModel,
		`SELECT leasedUntil FROM crlShards WHERE issuerID = ? AND idx = ? LIMIT 1`,
		res.IssuerNameID,
		res.ShardIdx,
	)
	test.AssertNotError(t, err, "getting updated lease timestamp")
	test.Assert(t, untilModel.LeasedUntil.Equal(until), "checking updated lease timestamp")

	// Leasing a previously-unknown specific shard should work (to ease the
	// transition into using leasing).
	res, err = sa.leaseSpecificCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 1,
			MinShardIdx:  9,
			MaxShardIdx:  9,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertNotError(t, err, "leasing unknown shard")

	err = sa.dbMap.SelectOne(
		ctx,
		&untilModel,
		`SELECT leasedUntil FROM crlShards WHERE issuerID = ? AND idx = ? LIMIT 1`,
		res.IssuerNameID,
		res.ShardIdx,
	)
	test.AssertNotError(t, err, "getting updated lease timestamp")
	test.Assert(t, untilModel.LeasedUntil.Equal(until), "checking updated lease timestamp")

	// Leasing a leased shard should fail.
	_, err = sa.leaseSpecificCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 1,
			MinShardIdx:  0,
			MaxShardIdx:  0,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertError(t, err, "leasing unavailable shard")

	// Leasing more than one shard should fail.
	_, err = sa.leaseSpecificCRLShard(
		context.Background(),
		&sapb.LeaseCRLShardRequest{
			IssuerNameID: 1,
			MinShardIdx:  1,
			MaxShardIdx:  2,
			Until:        timestamppb.New(until),
		},
	)
	test.AssertError(t, err, "did not lease one specific shard")
}

func TestUpdateCRLShard(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	// Create 8 shards: 4 for each of 2 issuers. For each issuer, one shard is
	// currently leased, three are available, and one of those failed to update.
	_, err := sa.dbMap.ExecContext(ctx,
		`INSERT INTO crlShards (issuerID, idx, thisUpdate, nextUpdate, leasedUntil) VALUES
		(1, 0, ?, ?, ?),
		(1, 1, ?, ?, ?),
		(1, 2, ?, ?, ?),
		(1, 3, NULL, NULL, ?),
		(2, 0, ?, ?, ?),
		(2, 1, ?, ?, ?),
		(2, 2, ?, ?, ?),
		(2, 3, NULL, NULL, ?);`,
		clk.Now().Add(-7*24*time.Hour), clk.Now().Add(3*24*time.Hour), clk.Now().Add(time.Hour),
		clk.Now().Add(-6*24*time.Hour), clk.Now().Add(4*24*time.Hour), clk.Now().Add(-6*24*time.Hour),
		clk.Now().Add(-5*24*time.Hour), clk.Now().Add(5*24*time.Hour), clk.Now().Add(-5*24*time.Hour),
		clk.Now().Add(-4*24*time.Hour),
		clk.Now().Add(-7*24*time.Hour), clk.Now().Add(3*24*time.Hour), clk.Now().Add(time.Hour),
		clk.Now().Add(-6*24*time.Hour), clk.Now().Add(4*24*time.Hour), clk.Now().Add(-6*24*time.Hour),
		clk.Now().Add(-5*24*time.Hour), clk.Now().Add(5*24*time.Hour), clk.Now().Add(-5*24*time.Hour),
		clk.Now().Add(-4*24*time.Hour),
	)
	test.AssertNotError(t, err, "setting up test shards")

	thisUpdate := clk.Now().Truncate(time.Second).UTC()
	var crlModel struct {
		ThisUpdate *time.Time
		NextUpdate *time.Time
	}

	// Updating a leased shard should work.
	_, err = sa.UpdateCRLShard(
		context.Background(),
		&sapb.UpdateCRLShardRequest{
			IssuerNameID: 1,
			ShardIdx:     0,
			ThisUpdate:   timestamppb.New(thisUpdate),
			NextUpdate:   timestamppb.New(thisUpdate.Add(10 * 24 * time.Hour)),
		},
	)
	test.AssertNotError(t, err, "updating leased shard")

	err = sa.dbMap.SelectOne(
		ctx,
		&crlModel,
		`SELECT thisUpdate FROM crlShards WHERE issuerID = 1 AND idx = 0 LIMIT 1`,
	)
	test.AssertNotError(t, err, "getting updated thisUpdate timestamp")
	test.AssertEquals(t, *crlModel.ThisUpdate, thisUpdate)

	// Updating an unleased shard should work.
	_, err = sa.UpdateCRLShard(
		context.Background(),
		&sapb.UpdateCRLShardRequest{
			IssuerNameID: 1,
			ShardIdx:     1,
			ThisUpdate:   timestamppb.New(thisUpdate),
			NextUpdate:   timestamppb.New(thisUpdate.Add(10 * 24 * time.Hour)),
		},
	)
	test.AssertNotError(t, err, "updating unleased shard")

	err = sa.dbMap.SelectOne(
		ctx,
		&crlModel,
		`SELECT thisUpdate FROM crlShards WHERE issuerID = 1 AND idx = 1 LIMIT 1`,
	)
	test.AssertNotError(t, err, "getting updated thisUpdate timestamp")
	test.Assert(t, crlModel.ThisUpdate.Equal(thisUpdate), "checking updated thisUpdate timestamp")

	// Updating without supplying a NextUpdate should work.
	_, err = sa.UpdateCRLShard(
		context.Background(),
		&sapb.UpdateCRLShardRequest{
			IssuerNameID: 1,
			ShardIdx:     3,
			ThisUpdate:   timestamppb.New(thisUpdate.Add(time.Second)),
		},
	)
	test.AssertNotError(t, err, "updating shard without NextUpdate")

	err = sa.dbMap.SelectOne(
		ctx,
		&crlModel,
		`SELECT nextUpdate FROM crlShards WHERE issuerID = 1 AND idx = 3 LIMIT 1`,
	)
	test.AssertNotError(t, err, "getting updated nextUpdate timestamp")
	test.AssertBoxedNil(t, crlModel.NextUpdate, "checking updated nextUpdate timestamp")

	// Updating a shard to an earlier time should fail.
	_, err = sa.UpdateCRLShard(
		context.Background(),
		&sapb.UpdateCRLShardRequest{
			IssuerNameID: 1,
			ShardIdx:     1,
			ThisUpdate:   timestamppb.New(thisUpdate.Add(-24 * time.Hour)),
			NextUpdate:   timestamppb.New(thisUpdate.Add(9 * 24 * time.Hour)),
		},
	)
	test.AssertError(t, err, "updating shard to an earlier time")

	// Updating an unknown shard should fail.
	_, err = sa.UpdateCRLShard(
		context.Background(),
		&sapb.UpdateCRLShardRequest{
			IssuerNameID: 1,
			ShardIdx:     4,
			ThisUpdate:   timestamppb.New(thisUpdate),
			NextUpdate:   timestamppb.New(thisUpdate.Add(10 * 24 * time.Hour)),
		},
	)
	test.AssertError(t, err, "updating an unknown shard")
}

func TestReplacementOrderExists(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires replacementOrders database table")
	}

	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	features.Set(features.Config{TrackReplacementCertificatesARI: true})
	defer features.Reset()

	oldCertSerial := "1234567890"

	// Check that a non-existent replacement order does not exist.
	exists, err := sa.ReplacementOrderExists(ctx, &sapb.Serial{Serial: oldCertSerial})
	test.AssertNotError(t, err, "failed to check for replacement order")
	test.Assert(t, !exists.Exists, "replacement for non-existent serial should not exist")

	// Create a test registration to reference.
	reg := createWorkingRegistration(t, sa)

	// Add one valid authz.
	expires := fc.Now().Add(time.Hour)
	attemptedAt := fc.Now()
	authzID := createFinalizedAuthorization(t, sa, "example.com", expires, "valid", attemptedAt)

	// Add a new order in pending status with no certificate serial.
	expires1Year := sa.clk.Now().Add(365 * 24 * time.Hour)
	order, err := sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Names:            []string{"example.com"},
			V2Authorizations: []int64{authzID},
		},
	})
	test.AssertNotError(t, err, "NewOrderAndAuthzs failed")

	// Set the order to processing so it can be finalized
	_, err = sa.SetOrderProcessing(ctx, &sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "SetOrderProcessing failed")

	// Finalize the order with a certificate oldCertSerial.
	order.CertificateSerial = oldCertSerial
	_, err = sa.FinalizeOrder(ctx, &sapb.FinalizeOrderRequest{Id: order.Id, CertificateSerial: order.CertificateSerial})
	test.AssertNotError(t, err, "FinalizeOrder failed")

	// Create a replacement order.
	order, err = sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Names:            []string{"example.com"},
			V2Authorizations: []int64{authzID},
			ReplacesSerial:   oldCertSerial,
		},
	})
	test.AssertNotError(t, err, "NewOrderAndAuthzs failed")

	// Check that a pending replacement order exists.
	exists, err = sa.ReplacementOrderExists(ctx, &sapb.Serial{Serial: oldCertSerial})
	test.AssertNotError(t, err, "failed to check for replacement order")
	test.Assert(t, exists.Exists, "replacement order should exist")

	// Set the order to processing so it can be finalized.
	_, err = sa.SetOrderProcessing(ctx, &sapb.OrderRequest{Id: order.Id})
	test.AssertNotError(t, err, "SetOrderProcessing failed")

	// Check that a replacement order in processing still exists.
	exists, err = sa.ReplacementOrderExists(ctx, &sapb.Serial{Serial: oldCertSerial})
	test.AssertNotError(t, err, "failed to check for replacement order")
	test.Assert(t, exists.Exists, "replacement order in processing should still exist")

	order.CertificateSerial = "0123456789"
	_, err = sa.FinalizeOrder(ctx, &sapb.FinalizeOrderRequest{Id: order.Id, CertificateSerial: order.CertificateSerial})
	test.AssertNotError(t, err, "FinalizeOrder failed")

	// Check that a finalized replacement order still exists.
	exists, err = sa.ReplacementOrderExists(ctx, &sapb.Serial{Serial: oldCertSerial})
	test.AssertNotError(t, err, "failed to check for replacement order")
	test.Assert(t, exists.Exists, "replacement order in processing should still exist")

	// Try updating the replacement order.

	// Create a replacement order.
	newReplacementOrder, err := sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Names:            []string{"example.com"},
			V2Authorizations: []int64{authzID},
			ReplacesSerial:   oldCertSerial,
		},
	})
	test.AssertNotError(t, err, "NewOrderAndAuthzs failed")

	// Fetch the replacement order so we can ensure it was updated.
	var replacementRow replacementOrderModel
	err = sa.dbReadOnlyMap.SelectOne(
		ctx,
		&replacementRow,
		"SELECT * FROM replacementOrders WHERE serial = ? LIMIT 1",
		oldCertSerial,
	)
	test.AssertNotError(t, err, "SELECT from replacementOrders failed")
	test.AssertEquals(t, newReplacementOrder.Id, replacementRow.OrderID)
	test.AssertEquals(t, newReplacementOrder.Expires.AsTime(), replacementRow.OrderExpires)
}

func TestGetSerialsByKey(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	// Insert four rows into keyHashToSerial: two that should match the query,
	// one that should not match due to keyHash mismatch, and one that should not
	// match due to being already expired.
	expectedHash := make([]byte, 32)
	expectedHash[0] = 1
	differentHash := make([]byte, 32)
	differentHash[0] = 2
	inserts := []keyHashModel{
		{
			KeyHash:      expectedHash,
			CertSerial:   "1",
			CertNotAfter: fc.Now().Add(time.Hour),
		},
		{
			KeyHash:      expectedHash,
			CertSerial:   "2",
			CertNotAfter: fc.Now().Add(2 * time.Hour),
		},
		{
			KeyHash:      expectedHash,
			CertSerial:   "3",
			CertNotAfter: fc.Now().Add(-1 * time.Hour),
		},
		{
			KeyHash:      differentHash,
			CertSerial:   "4",
			CertNotAfter: fc.Now().Add(time.Hour),
		},
	}

	for _, row := range inserts {
		err := sa.dbMap.Insert(context.Background(), &row)
		test.AssertNotError(t, err, "inserting test keyHash")
	}

	// Expect the result res to have two entries.
	res := make(chan *sapb.Serial)
	stream := &fakeServerStream[sapb.Serial]{output: res}
	var err error
	go func() {
		err = sa.GetSerialsByKey(&sapb.SPKIHash{KeyHash: expectedHash}, stream)
		close(res) // Let our main test thread continue.
	}()

	var seen []string
	for serial := range res {
		if !slices.Contains([]string{"1", "2"}, serial.Serial) {
			t.Errorf("Received unexpected serial %q", serial.Serial)
		}
		if slices.Contains(seen, serial.Serial) {
			t.Errorf("Received serial %q more than once", serial.Serial)
		}
		seen = append(seen, serial.Serial)
	}
	test.AssertNotError(t, err, "calling GetSerialsByKey")
	test.AssertEquals(t, len(seen), 2)
}

func TestGetSerialsByAccount(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	expectedReg := createWorkingRegistration(t, sa)

	// Insert three rows into the serials table: two that should match the query,
	// and one that should not match due to being already expired. We do not here
	// test filtering on the regID itself, because our test setup makes it very
	// hard to insert two fake registrations rows with different IDs.
	inserts := []recordedSerialModel{
		{
			Serial:         "1",
			RegistrationID: expectedReg.Id,
			Created:        fc.Now().Add(-23 * time.Hour),
			Expires:        fc.Now().Add(time.Hour),
		},
		{
			Serial:         "2",
			RegistrationID: expectedReg.Id,
			Created:        fc.Now().Add(-22 * time.Hour),
			Expires:        fc.Now().Add(2 * time.Hour),
		},
		{
			Serial:         "3",
			RegistrationID: expectedReg.Id,
			Created:        fc.Now().Add(-23 * time.Hour),
			Expires:        fc.Now().Add(-1 * time.Hour),
		},
	}

	for _, row := range inserts {
		err := sa.dbMap.Insert(context.Background(), &row)
		test.AssertNotError(t, err, "inserting test serial")
	}

	// Expect the result stream to have two entries.
	res := make(chan *sapb.Serial)
	stream := &fakeServerStream[sapb.Serial]{output: res}
	var err error
	go func() {
		err = sa.GetSerialsByAccount(&sapb.RegistrationID{Id: expectedReg.Id}, stream)
		close(res) // Let our main test thread continue.
	}()

	var seen []string
	for serial := range res {
		if !slices.Contains([]string{"1", "2"}, serial.Serial) {
			t.Errorf("Received unexpected serial %q", serial.Serial)
		}
		if slices.Contains(seen, serial.Serial) {
			t.Errorf("Received serial %q more than once", serial.Serial)
		}
		seen = append(seen, serial.Serial)
	}
	test.AssertNotError(t, err, "calling GetSerialsByAccount")
	test.AssertEquals(t, len(seen), 2)
}

func TestUnpauseAccount(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires paused database table")
	}
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	tests := []struct {
		name  string
		state []pausedModel
		req   *sapb.RegistrationID
	}{
		{
			name:  "UnpauseAccount with no paused identifiers",
			state: nil,
			req:   &sapb.RegistrationID{Id: 1},
		},
		{
			name: "UnpauseAccount with one paused identifier",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
			},
			req: &sapb.RegistrationID{Id: 1},
		},
		{
			name: "UnpauseAccount with multiple paused identifiers",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.net",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.org",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
			},
			req: &sapb.RegistrationID{Id: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				// Drop all rows from the paused table.
				_, err := sa.dbMap.ExecContext(ctx, "TRUNCATE TABLE paused")
				test.AssertNotError(t, err, "truncating paused table")
			}()

			// Setup table state.
			for _, state := range tt.state {
				err := sa.dbMap.Insert(ctx, &state)
				test.AssertNotError(t, err, "inserting test identifier")
			}

			_, err := sa.UnpauseAccount(ctx, tt.req)
			test.AssertNotError(t, err, "Unexpected error for UnpauseAccount()")

			// Count the number of paused identifiers.
			var count int
			err = sa.dbReadOnlyMap.SelectOne(
				ctx,
				&count,
				"SELECT COUNT(*) FROM paused WHERE registrationID = ? AND unpausedAt IS NULL",
				tt.req.Id,
			)
			test.AssertNotError(t, err, "SELECT COUNT(*) failed")
			test.AssertEquals(t, count, 0)
		})
	}
}

func TestPauseIdentifiers(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires paused database table")
	}
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	ptrTime := func(t time.Time) *time.Time {
		return &t
	}

	tests := []struct {
		name  string
		state []pausedModel
		req   *sapb.PauseRequest
		want  *sapb.PauseIdentifiersResponse
	}{
		{
			name:  "An identifier which is not now or previously paused",
			state: nil,
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
				},
			},
			want: &sapb.PauseIdentifiersResponse{
				Paused:   1,
				Repaused: 0,
			},
		},
		{
			name: "One unpaused entry which was previously paused",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt:   sa.clk.Now().Add(-time.Hour),
					UnpausedAt: ptrTime(sa.clk.Now().Add(-time.Minute)),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
				},
			},
			want: &sapb.PauseIdentifiersResponse{
				Paused:   0,
				Repaused: 1,
			},
		},
		{
			name: "An identifier which is currently paused",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
				},
			},
			want: &sapb.PauseIdentifiersResponse{
				Paused:   0,
				Repaused: 0,
			},
		},
		{
			name: "Two previously paused entries and one new entry",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt:   sa.clk.Now().Add(-time.Hour),
					UnpausedAt: ptrTime(sa.clk.Now().Add(-time.Minute)),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.net",
					},
					PausedAt:   sa.clk.Now().Add(-time.Hour),
					UnpausedAt: ptrTime(sa.clk.Now().Add(-time.Minute)),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.DNS),
						Value: "example.net",
					},
					{
						Type:  string(identifier.DNS),
						Value: "example.org",
					},
				},
			},
			want: &sapb.PauseIdentifiersResponse{
				Paused:   1,
				Repaused: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				// Drop all rows from the paused table.
				_, err := sa.dbMap.ExecContext(ctx, "TRUNCATE TABLE paused")
				test.AssertNotError(t, err, "Truncate table paused failed")
			}()

			// Setup table state.
			for _, state := range tt.state {
				err := sa.dbMap.Insert(ctx, &state)
				test.AssertNotError(t, err, "inserting test identifier")
			}

			got, err := sa.PauseIdentifiers(ctx, tt.req)
			test.AssertNotError(t, err, "Unexpected error for PauseIdentifiers()")
			test.AssertEquals(t, got.Paused, tt.want.Paused)
			test.AssertEquals(t, got.Repaused, tt.want.Repaused)
		})
	}
}

func TestCheckIdentifiersPaused(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires paused database table")
	}
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	ptrTime := func(t time.Time) *time.Time {
		return &t
	}

	tests := []struct {
		name  string
		state []pausedModel
		req   *sapb.PauseRequest
		want  *sapb.Identifiers
	}{
		{
			name:  "No paused identifiers",
			state: nil,
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
				},
			},
			want: &sapb.Identifiers{
				Identifiers: []*sapb.Identifier{},
			},
		},
		{
			name: "One paused identifier",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
				},
			},
			want: &sapb.Identifiers{
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
				},
			},
		},
		{
			name: "Two paused identifiers, one unpaused",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.net",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.org",
					},
					PausedAt:   sa.clk.Now().Add(-time.Hour),
					UnpausedAt: ptrTime(sa.clk.Now().Add(-time.Minute)),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.DNS),
						Value: "example.net",
					},
					{
						Type:  string(identifier.DNS),
						Value: "example.org",
					},
				},
			},
			want: &sapb.Identifiers{
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.DNS),
						Value: "example.net",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				// Drop all rows from the paused table.
				_, err := sa.dbMap.ExecContext(ctx, "TRUNCATE TABLE paused")
				test.AssertNotError(t, err, "Truncate table paused failed")
			}()

			// Setup table state.
			for _, state := range tt.state {
				err := sa.dbMap.Insert(ctx, &state)
				test.AssertNotError(t, err, "inserting test identifier")
			}

			got, err := sa.CheckIdentifiersPaused(ctx, tt.req)
			test.AssertNotError(t, err, "Unexpected error for PauseIdentifiers()")
			test.AssertDeepEquals(t, got.Identifiers, tt.want.Identifiers)
		})
	}
}

func TestGetPausedIdentifiers(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires paused database table")
	}
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	ptrTime := func(t time.Time) *time.Time {
		return &t
	}

	tests := []struct {
		name  string
		state []pausedModel
		req   *sapb.RegistrationID
		want  *sapb.Identifiers
	}{
		{
			name:  "No paused identifiers",
			state: nil,
			req:   &sapb.RegistrationID{Id: 1},
			want: &sapb.Identifiers{
				Identifiers: []*sapb.Identifier{},
			},
		},
		{
			name: "One paused identifier",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
			},
			req: &sapb.RegistrationID{Id: 1},
			want: &sapb.Identifiers{
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
				},
			},
		},
		{
			name: "Two paused identifiers, one unpaused",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.net",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.DNS)],
						Value: "example.org",
					},
					PausedAt:   sa.clk.Now().Add(-time.Hour),
					UnpausedAt: ptrTime(sa.clk.Now().Add(-time.Minute)),
				},
			},
			req: &sapb.RegistrationID{Id: 1},
			want: &sapb.Identifiers{
				Identifiers: []*sapb.Identifier{
					{
						Type:  string(identifier.DNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.DNS),
						Value: "example.net",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				// Drop all rows from the paused table.
				_, err := sa.dbMap.ExecContext(ctx, "TRUNCATE TABLE paused")
				test.AssertNotError(t, err, "Truncate table paused failed")
			}()

			// Setup table state.
			for _, state := range tt.state {
				err := sa.dbMap.Insert(ctx, &state)
				test.AssertNotError(t, err, "inserting test identifier")
			}

			got, err := sa.GetPausedIdentifiers(ctx, tt.req)
			test.AssertNotError(t, err, "Unexpected error for PauseIdentifiers()")
			test.AssertDeepEquals(t, got.Identifiers, tt.want.Identifiers)
		})
	}
}

func TestGetPausedIdentifiersOnlyUnpausesOneAccount(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		t.Skip("Test requires paused database table")
	}
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	// Insert two paused identifiers for two different accounts.
	err := sa.dbMap.Insert(ctx, &pausedModel{
		RegistrationID: 1,
		identifierModel: identifierModel{
			Type:  identifierTypeToUint[string(identifier.DNS)],
			Value: "example.com",
		},
		PausedAt: sa.clk.Now().Add(-time.Hour),
	})
	test.AssertNotError(t, err, "inserting test identifier")

	err = sa.dbMap.Insert(ctx, &pausedModel{
		RegistrationID: 2,
		identifierModel: identifierModel{
			Type:  identifierTypeToUint[string(identifier.DNS)],
			Value: "example.net",
		},
		PausedAt: sa.clk.Now().Add(-time.Hour),
	})
	test.AssertNotError(t, err, "inserting test identifier")

	// Unpause the first account.
	_, err = sa.UnpauseAccount(ctx, &sapb.RegistrationID{Id: 1})
	test.AssertNotError(t, err, "UnpauseAccount failed")

	// Check that the second account's identifier is still paused.
	identifiers, err := sa.GetPausedIdentifiers(ctx, &sapb.RegistrationID{Id: 2})
	test.AssertNotError(t, err, "GetPausedIdentifiers failed")
	test.AssertEquals(t, len(identifiers.Identifiers), 1)
	test.AssertEquals(t, identifiers.Identifiers[0].Value, "example.net")
}
