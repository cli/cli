package sa

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"math/bits"
	mrand "math/rand/v2"
	"net/netip"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
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

func mustTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(fmt.Sprintf("parsing %q: %s", s, err))
	}
	return t.UTC()
}

func mustTimestamp(s string) *timestamppb.Timestamp {
	return timestamppb.New(mustTime(s))
}

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
func initSA(t testing.TB) (*SQLStorageAuthority, clock.FakeClock, func()) {
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
	fc.Set(mustTime("2015-03-04 05:00"))

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
func createWorkingRegistration(t testing.TB, sa *SQLStorageAuthority) *corepb.Registration {
	reg, err := sa.NewRegistration(context.Background(), &corepb.Registration{
		Key:       []byte(theKey),
		Contact:   []string{"mailto:foo@example.com"},
		CreatedAt: mustTimestamp("2003-05-10 00:00"),
		Status:    string(core.StatusValid),
	})
	if err != nil {
		t.Fatalf("Unable to create new registration: %s", err)
	}
	return reg
}

func createPendingAuthorization(t *testing.T, sa *SQLStorageAuthority, ident identifier.ACMEIdentifier, exp time.Time) int64 {
	t.Helper()

	tokenStr := core.NewToken()
	token, err := base64.RawURLEncoding.DecodeString(tokenStr)
	test.AssertNotError(t, err, "computing test authorization challenge token")

	am := authzModel{
		IdentifierType:  identifierTypeToUint[string(ident.Type)],
		IdentifierValue: ident.Value,
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

func createFinalizedAuthorization(t *testing.T, sa *SQLStorageAuthority, ident identifier.ACMEIdentifier, exp time.Time,
	status string, attemptedAt time.Time) int64 {
	t.Helper()
	pendingID := createPendingAuthorization(t, sa, ident, exp)
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

	jwkJSON, _ := goodTestJWK().MarshalJSON()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:     jwkJSON,
		Contact: []string{"mailto:foo@example.com"},
	})
	if err != nil {
		t.Fatalf("Couldn't create new registration: %s", err)
	}
	test.Assert(t, reg.Id != 0, "ID shouldn't be 0")
	test.AssertEquals(t, len(reg.Contact), 0)

	// Confirm that the registration can be retrieved by ID.
	dbReg, err := sa.GetRegistration(ctx, &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, fmt.Sprintf("Couldn't get registration with ID %v", reg.Id))

	createdAt := clk.Now()
	test.AssertEquals(t, dbReg.Id, reg.Id)
	test.AssertByteEquals(t, dbReg.Key, jwkJSON)
	test.AssertDeepEquals(t, dbReg.CreatedAt.AsTime(), createdAt)
	test.AssertEquals(t, len(dbReg.Contact), 0)

	_, err = sa.GetRegistration(ctx, &sapb.RegistrationID{Id: 0})
	test.AssertError(t, err, "Registration object for ID 0 was returned")

	// Confirm that the registration can be retrieved by key.
	dbReg, err = sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: jwkJSON})
	test.AssertNotError(t, err, "Couldn't get registration by key")
	test.AssertEquals(t, dbReg.Id, dbReg.Id)
	test.AssertEquals(t, dbReg.Agreement, dbReg.Agreement)

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

	_, err = sa.UpdateRegistrationKey(ctx, &sapb.UpdateRegistrationKeyRequest{RegistrationID: 100, Jwk: jwkJSON})
	test.AssertErrorIs(t, err, berrors.InternalServer)
}

func TestSelectRegistration(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()
	var ctx = context.Background()
	jwk := goodTestJWK()
	jwkJSON, _ := jwk.MarshalJSON()
	sha, err := core.KeyDigestB64(jwk.Key)
	test.AssertNotError(t, err, "couldn't parse jwk.Key")

	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key:     jwkJSON,
		Contact: []string{"mailto:foo@example.com"},
	})
	test.AssertNotError(t, err, fmt.Sprintf("couldn't create new registration: %s", err))
	test.Assert(t, reg.Id != 0, "ID shouldn't be 0")

	_, err = selectRegistration(ctx, sa.dbMap, "id", reg.Id)
	test.AssertNotError(t, err, "selecting by id should work")
	_, err = selectRegistration(ctx, sa.dbMap, "jwk_sha256", sha)
	test.AssertNotError(t, err, "selecting by jwk_sha256 should work")
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
func findIssuedName(ctx context.Context, dbMap db.OneSelector, issuedName string) (string, error) {
	var issuedNamesSerial string
	err := dbMap.SelectOne(
		ctx,
		&issuedNamesSerial,
		`SELECT serial FROM issuedNames
		WHERE reversedName = ?
		ORDER BY notBefore DESC
		LIMIT 1`,
		issuedName)
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
	issuedTime := mustTimestamp("2018-04-01 07:00")
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        regID,
		Issued:       issuedTime,
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
	issuedNamesSerial, err := findIssuedName(ctx, sa.dbMap, reverseFQDN(testCert.DNSNames[0]))
	test.AssertNotError(t, err, "expected no err querying issuedNames for precert")
	test.AssertEquals(t, issuedNamesSerial, serial)

	// We should also be able to call AddCertificate with the same cert
	// without it being an error. The duplicate err on inserting to
	// issuedNames should be ignored.
	_, err = sa.AddCertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  regID,
		Issued: issuedTime,
	})
	test.AssertNotError(t, err, "unexpected err adding final cert after precert")
}

func TestAddPrecertificateNoOCSP(t *testing.T) {
	sa, clk, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)
	_, testCert := test.ThrowAwayCert(t, clk)

	regID := reg.Id
	issuedTime := mustTimestamp("2018-04-01 07:00")
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:          testCert.Raw,
		RegID:        regID,
		Issued:       issuedTime,
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
	_, err := sa.AddPrecertificate(ctx, &sapb.AddCertificateRequest{
		Der:    testCert.Raw,
		RegID:  regID,
		Issued: mustTimestamp("2018-04-01 07:00"),
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

func TestFQDNSetTimestampsForWindow(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	tx, err := sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")

	idents := identifier.ACMEIdentifiers{
		identifier.NewDNS("a.example.com"),
		identifier.NewDNS("B.example.com"),
	}

	// Invalid Window
	req := &sapb.CountFQDNSetsRequest{
		Identifiers: idents.ToProtoSlice(),
		Window:      nil,
	}
	_, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertErrorIs(t, err, errIncompleteRequest)

	window := time.Hour * 3
	req = &sapb.CountFQDNSetsRequest{
		Identifiers: idents.ToProtoSlice(),
		Window:      durationpb.New(window),
	}

	// Ensure zero issuance has occurred for names.
	resp, err := sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 0)

	// Add an issuance for names inside the window.
	expires := fc.Now().Add(time.Hour * 2).UTC()
	firstIssued := fc.Now()
	err = addFQDNSet(ctx, tx, idents, "serial", firstIssued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// Ensure there's 1 issuance timestamp for names inside the window.
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 1)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())

	// Ensure that the hash isn't affected by changing name order/casing.
	req.Identifiers = []*corepb.Identifier{
		identifier.NewDNS("b.example.com").ToProto(),
		identifier.NewDNS("A.example.COM").ToProto(),
	}
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 1)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())

	// Add another issuance for names inside the window.
	tx, err = sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	err = addFQDNSet(ctx, tx, idents, "anotherSerial", firstIssued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// Ensure there are two issuance timestamps for names inside the window.
	req.Identifiers = idents.ToProtoSlice()
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 2)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())

	// Add another issuance for names but just outside the window.
	tx, err = sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	err = addFQDNSet(ctx, tx, idents, "yetAnotherSerial", firstIssued.Add(-window), expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	// Ensure there are still only two issuance timestamps in the window.
	resp, err = sa.FQDNSetTimestampsForWindow(ctx, req)
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 2)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())

	resp, err = sa.FQDNSetTimestampsForWindow(ctx, &sapb.CountFQDNSetsRequest{
		Identifiers: idents.ToProtoSlice(),
		Window:      durationpb.New(window),
		Limit:       1,
	})
	test.AssertNotError(t, err, "Failed to count name sets")
	test.AssertEquals(t, len(resp.Timestamps), 1)
	test.AssertEquals(t, firstIssued, resp.Timestamps[len(resp.Timestamps)-1].AsTime())
}

func TestFQDNSetExists(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	idents := identifier.ACMEIdentifiers{
		identifier.NewDNS("a.example.com"),
		identifier.NewDNS("B.example.com"),
	}

	exists, err := sa.FQDNSetExists(ctx, &sapb.FQDNSetExistsRequest{Identifiers: idents.ToProtoSlice()})
	test.AssertNotError(t, err, "Failed to check FQDN set existence")
	test.Assert(t, !exists.Exists, "FQDN set shouldn't exist")

	tx, err := sa.dbMap.BeginTx(ctx)
	test.AssertNotError(t, err, "Failed to open transaction")
	expires := fc.Now().Add(time.Hour * 2).UTC()
	issued := fc.Now()
	err = addFQDNSet(ctx, tx, idents, "serial", issued, expires)
	test.AssertNotError(t, err, "Failed to add name set")
	test.AssertNotError(t, tx.Commit(), "Failed to commit transaction")

	exists, err = sa.FQDNSetExists(ctx, &sapb.FQDNSetExistsRequest{Identifiers: idents.ToProtoSlice()})
	test.AssertNotError(t, err, "Failed to check FQDN set existence")
	test.Assert(t, exists.Exists, "FQDN set does exist")
}

type execRecorder struct {
	valuesPerRow int
	query        string
	args         []interface{}
}

func (e *execRecorder) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	e.query = query
	e.args = args
	return rowsResult{int64(len(args) / e.valuesPerRow)}, nil
}

type rowsResult struct {
	rowsAffected int64
}

func (r rowsResult) LastInsertId() (int64, error) {
	return r.rowsAffected, nil
}

func (r rowsResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

func TestAddIssuedNames(t *testing.T) {
	serial := big.NewInt(1)
	expectedSerial := "000000000000000000000000000000000001"
	notBefore := mustTime("2018-02-14 12:00")
	expectedNotBefore := notBefore.Truncate(24 * time.Hour)
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
				expectedNotBefore,
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
				expectedNotBefore,
				false,
				"xyz.example",
				expectedSerial,
				expectedNotBefore,
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
				expectedNotBefore,
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
				expectedNotBefore,
				true,
				"xyz.example",
				expectedSerial,
				expectedNotBefore,
				true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			e := execRecorder{valuesPerRow: 4}
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
	authzID := createPendingAuthorization(t, sa, identifier.NewDNS("example.com"), expires)
	_, err := sa.DeactivateAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "sa.DeactivateAuthorization2 failed")

	// deactivate a valid authorization
	authzID = createFinalizedAuthorization(t, sa, identifier.NewDNS("example.com"), expires, "valid", attemptedAt)
	_, err = sa.DeactivateAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: authzID})
	test.AssertNotError(t, err, "sa.DeactivateAuthorization2 failed")
}

func TestDeactivateAccount(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	reg := createWorkingRegistration(t, sa)

	// An incomplete request should be rejected.
	_, err := sa.DeactivateRegistration(context.Background(), &sapb.RegistrationID{})
	test.AssertError(t, err, "Incomplete request should fail")
	test.AssertContains(t, err.Error(), "incomplete")

	// Deactivating should work, and return the same account but with updated
	// status and cleared contacts.
	got, err := sa.DeactivateRegistration(context.Background(), &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, "DeactivateRegistration failed")
	test.AssertEquals(t, got.Id, reg.Id)
	test.AssertEquals(t, core.AcmeStatus(got.Status), core.StatusDeactivated)
	test.AssertEquals(t, len(got.Contact), 0)

	// Double-check that the DeactivateRegistration method returned the right
	// thing, by fetching the same account ourselves.
	got, err = sa.GetRegistration(context.Background(), &sapb.RegistrationID{Id: reg.Id})
	test.AssertNotError(t, err, "GetRegistration failed")
	test.AssertEquals(t, got.Id, reg.Id)
	test.AssertEquals(t, core.AcmeStatus(got.Status), core.StatusDeactivated)
	test.AssertEquals(t, len(got.Contact), 0)

	// Attempting to deactivate it a second time should fail, since it is already
	// deactivated.
	_, err = sa.DeactivateRegistration(context.Background(), &sapb.RegistrationID{Id: reg.Id})
	test.AssertError(t, err, "Deactivating an already-deactivated account should fail")
}

func TestReverseFQDN(t *testing.T) {
	testCases := []struct {
		fqdn     string
		reversed string
	}{
		{"", ""},
		{"...", "..."},
		{"com", "com"},
		{"example.com", "com.example"},
		{"www.example.com", "com.example.www"},
		{"world.wide.web.example.com", "com.example.web.wide.world"},
	}

	for _, tc := range testCases {
		output := reverseFQDN(tc.fqdn)
		test.AssertEquals(t, output, tc.reversed)

		output = reverseFQDN(tc.reversed)
		test.AssertEquals(t, output, tc.fqdn)
	}
}

func TestEncodeIssuedName(t *testing.T) {
	testCases := []struct {
		issuedName string
		reversed   string
		oneWay     bool
	}{
		// Empty strings and bare separators/TLDs should be unchanged.
		{"", "", false},
		{"...", "...", false},
		{"com", "com", false},
		// FQDNs should be reversed.
		{"example.com", "com.example", false},
		{"www.example.com", "com.example.www", false},
		{"world.wide.web.example.com", "com.example.web.wide.world", false},
		// IP addresses should stay the same.
		{"1.2.3.4", "1.2.3.4", false},
		{"2602:ff3a:1:abad:c0f:fee:abad:cafe", "2602:ff3a:1:abad:c0f:fee:abad:cafe", false},
		// Tricksy FQDNs that look like IPv6 addresses should be parsed as FQDNs.
		{"2602.ff3a.1.abad.c0f.fee.abad.cafe", "cafe.abad.fee.c0f.abad.1.ff3a.2602", false},
		{"2602.ff3a.0001.abad.0c0f.0fee.abad.cafe", "cafe.abad.0fee.0c0f.abad.0001.ff3a.2602", false},
		// IPv6 addresses should be returned in RFC 5952 format.
		{"2602:ff3a:0001:abad:0c0f:0fee:abad:cafe", "2602:ff3a:1:abad:c0f:fee:abad:cafe", true},
	}

	for _, tc := range testCases {
		output := EncodeIssuedName(tc.issuedName)
		test.AssertEquals(t, output, tc.reversed)

		if !tc.oneWay {
			output = EncodeIssuedName(tc.reversed)
			test.AssertEquals(t, output, tc.issuedName)
		}
	}
}

func TestNewOrderAndAuthzs(t *testing.T) {
	sa, _, cleanup := initSA(t)
	defer cleanup()

	reg := createWorkingRegistration(t, sa)

	// Insert two pre-existing authorizations to reference
	idA := createPendingAuthorization(t, sa, identifier.NewDNS("a.com"), sa.clk.Now().Add(time.Hour))
	idB := createPendingAuthorization(t, sa, identifier.NewDNS("b.com"), sa.clk.Now().Add(time.Hour))
	test.AssertEquals(t, idA, int64(1))
	test.AssertEquals(t, idB, int64(2))

	nowC := sa.clk.Now().Add(time.Hour)
	nowD := sa.clk.Now().Add(time.Hour)
	expires := sa.clk.Now().Add(2 * time.Hour)
	req := &sapb.NewOrderAndAuthzsRequest{
		// Insert an order for four names, two of which already have authzs
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID: reg.Id,
			Expires:        timestamppb.New(expires),
			Identifiers: []*corepb.Identifier{
				identifier.NewDNS("a.com").ToProto(),
				identifier.NewDNS("b.com").ToProto(),
				identifier.NewDNS("c.com").ToProto(),
				identifier.NewDNS("d.com").ToProto(),
			},
			V2Authorizations: []int64{1, 2},
		},
		// And add new authorizations for the other two names.
		NewAuthzs: []*sapb.NewAuthzRequest{
			{
				Identifier:     &corepb.Identifier{Type: "dns", Value: "c.com"},
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(nowC),
				ChallengeTypes: []string{string(core.ChallengeTypeHTTP01)},
				Token:          core.NewToken(),
			},
			{
				Identifier:     &corepb.Identifier{Type: "dns", Value: "d.com"},
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(nowD),
				ChallengeTypes: []string{string(core.ChallengeTypeHTTP01)},
				Token:          core.NewToken(),
			},
		},
	}
	order, err := sa.NewOrderAndAuthzs(context.Background(), req)
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

	reg := createWorkingRegistration(t, sa)

	expires := fc.Now().Add(2 * time.Hour)
	_, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewAuthzs: []*sapb.NewAuthzRequest{
			{
				Identifier:     &corepb.Identifier{Type: "dns", Value: "c.com"},
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(expires),
				ChallengeTypes: []string{string(core.ChallengeTypeDNS01)},
				Token:          core.NewToken(),
			},
		},
	})
	test.AssertErrorIs(t, err, errIncompleteRequest)
}

func TestNewOrderAndAuthzs_MismatchedRegID(t *testing.T) {
	sa, _, cleanup := initSA(t)
	defer cleanup()

	_, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID: 1,
		},
		NewAuthzs: []*sapb.NewAuthzRequest{
			{
				RegistrationID: 2,
			},
		},
	})
	test.AssertError(t, err, "mismatched regIDs should fail")
	test.AssertContains(t, err.Error(), "same account")
}

func TestNewOrderAndAuthzs_NewAuthzExpectedFields(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	reg := createWorkingRegistration(t, sa)
	expires := fc.Now().Add(time.Hour)
	domain := "a.com"

	// Create an authz that does not yet exist in the database with some invalid
	// data smuggled in.
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewAuthzs: []*sapb.NewAuthzRequest{
			{
				Identifier:     &corepb.Identifier{Type: "dns", Value: domain},
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(expires),
				ChallengeTypes: []string{string(core.ChallengeTypeHTTP01)},
				Token:          core.NewToken(),
			},
		},
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID: reg.Id,
			Expires:        timestamppb.New(expires),
			Identifiers:    []*corepb.Identifier{identifier.NewDNS(domain).ToProto()},
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

func TestNewOrderAndAuthzs_Profile(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	reg := createWorkingRegistration(t, sa)
	expires := fc.Now().Add(time.Hour)

	// Create and order and authz while specifying a profile.
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:         reg.Id,
			Expires:                timestamppb.New(expires),
			Identifiers:            []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
			CertificateProfileName: "test",
		},
		NewAuthzs: []*sapb.NewAuthzRequest{
			{
				Identifier:     &corepb.Identifier{Type: "dns", Value: "example.com"},
				RegistrationID: reg.Id,
				Expires:        timestamppb.New(expires),
				ChallengeTypes: []string{string(core.ChallengeTypeHTTP01)},
				Token:          core.NewToken(),
			},
		},
	})
	if err != nil {
		t.Fatalf("inserting order and authzs: %s", err)
	}

	// Retrieve the order and check that the profile is correct.
	gotOrder, err := sa.GetOrder(context.Background(), &sapb.OrderRequest{Id: order.Id})
	if err != nil {
		t.Fatalf("retrieving inserted order: %s", err)
	}
	if gotOrder.CertificateProfileName != "test" {
		t.Errorf("order.CertificateProfileName = %v, want %v", gotOrder.CertificateProfileName, "test")
	}

	// Retrieve the authz and check that the profile is correct.
	// Safely get the authz for the order we created above.
	gotAuthz, err := sa.GetAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: order.V2Authorizations[0]})
	if err != nil {
		t.Fatalf("retrieving inserted authz: %s", err)
	}
	if gotAuthz.CertificateProfileName != "test" {
		t.Errorf("authz.CertificateProfileName = %v, want %v", gotAuthz.CertificateProfileName, "test")
	}
}

func TestSetOrderProcessing(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	reg := createWorkingRegistration(t, sa)

	// Add one valid authz
	expires := fc.Now().Add(time.Hour)
	attemptedAt := fc.Now()
	authzID := createFinalizedAuthorization(t, sa, identifier.NewDNS("example.com"), expires, "valid", attemptedAt)

	// Add a new order in pending status with no certificate serial
	expires1Year := sa.clk.Now().Add(365 * 24 * time.Hour)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Identifiers:      []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
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

	reg := createWorkingRegistration(t, sa)
	expires := fc.Now().Add(time.Hour)
	attemptedAt := fc.Now()
	authzID := createFinalizedAuthorization(t, sa, identifier.NewDNS("example.com"), expires, "valid", attemptedAt)

	// Add a new order in pending status with no certificate serial
	expires1Year := sa.clk.Now().Add(365 * 24 * time.Hour)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Identifiers:      []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
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

// TestGetOrder tests that round-tripping a simple order through
// NewOrderAndAuthzs and GetOrder has the expected result.
func TestGetOrder(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	reg := createWorkingRegistration(t, sa)
	ident := identifier.NewDNS("example.com")
	authzExpires := fc.Now().Add(time.Hour)
	authzID := createPendingAuthorization(t, sa, ident, authzExpires)

	// Set the order to expire in two hours
	expires := fc.Now().Add(2 * time.Hour)

	inputOrder := &corepb.Order{
		RegistrationID:   reg.Id,
		Expires:          timestamppb.New(expires),
		Identifiers:      []*corepb.Identifier{ident.ToProto()},
		V2Authorizations: []int64{authzID},
	}

	// Create the order
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   inputOrder.RegistrationID,
			Expires:          inputOrder.Expires,
			Identifiers:      inputOrder.Identifiers,
			V2Authorizations: inputOrder.V2Authorizations,
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	// The Order from GetOrder should match the following expected order
	created := sa.clk.Now()
	expectedOrder := &corepb.Order{
		// The registration ID, authorizations, expiry, and identifiers should match the
		// input to NewOrderAndAuthzs
		RegistrationID:   inputOrder.RegistrationID,
		V2Authorizations: inputOrder.V2Authorizations,
		Identifiers:      inputOrder.Identifiers,
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

// TestGetOrderWithProfile tests that round-tripping a simple order through
// NewOrderAndAuthzs and GetOrder has the expected result.
func TestGetOrderWithProfile(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	reg := createWorkingRegistration(t, sa)
	ident := identifier.NewDNS("example.com")
	authzExpires := fc.Now().Add(time.Hour)
	authzID := createPendingAuthorization(t, sa, ident, authzExpires)

	// Set the order to expire in two hours
	expires := fc.Now().Add(2 * time.Hour)

	inputOrder := &corepb.Order{
		RegistrationID:         reg.Id,
		Expires:                timestamppb.New(expires),
		Identifiers:            []*corepb.Identifier{ident.ToProto()},
		V2Authorizations:       []int64{authzID},
		CertificateProfileName: "tbiapb",
	}

	// Create the order
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:         inputOrder.RegistrationID,
			Expires:                inputOrder.Expires,
			Identifiers:            inputOrder.Identifiers,
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
		Identifiers:      inputOrder.Identifiers,
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

	identA := identifier.NewDNS("aaa")
	identB := identifier.NewDNS("bbb")
	identC := identifier.NewDNS("ccc")
	identD := identifier.NewIP(netip.MustParseAddr("10.10.10.10"))
	idents := identifier.ACMEIdentifiers{identA, identB, identC, identD}
	identE := identifier.NewDNS("ddd")

	createFinalizedAuthorization(t, sa, identA, exp, "valid", attemptedAt)
	createPendingAuthorization(t, sa, identB, exp)
	nearbyExpires := fc.Now().UTC().Add(time.Hour)
	createPendingAuthorization(t, sa, identC, nearbyExpires)
	createFinalizedAuthorization(t, sa, identD, exp, "valid", attemptedAt)

	// Set an expiry cut off of 1 day in the future similar to `RA.NewOrderAndAuthzs`. This
	// should exclude pending authorization C based on its nearbyExpires expiry
	// value.
	expiryCutoff := fc.Now().AddDate(0, 0, 1)
	// Get authorizations for the identifiers used above.
	authz, err := sa.GetAuthorizations2(context.Background(), &sapb.GetAuthorizationsRequest{
		RegistrationID: reg.Id,
		Identifiers:    idents.ToProtoSlice(),
		ValidUntil:     timestamppb.New(expiryCutoff),
	})
	// It should not fail
	test.AssertNotError(t, err, "sa.GetAuthorizations2 failed")
	// We should get back three authorizations since one of the four
	// authorizations created above expires too soon.
	test.AssertEquals(t, len(authz.Authzs), 3)

	// Get authorizations for the identifiers used above, and one that doesn't exist
	authz, err = sa.GetAuthorizations2(context.Background(), &sapb.GetAuthorizationsRequest{
		RegistrationID: reg.Id,
		Identifiers:    append(idents.ToProtoSlice(), identE.ToProto()),
		ValidUntil:     timestamppb.New(expiryCutoff),
	})
	// It should not fail
	test.AssertNotError(t, err, "sa.GetAuthorizations2 failed")
	// It should still return only three authorizations
	test.AssertEquals(t, len(authz.Authzs), 3)
}

func TestFasterGetOrderForNames(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	ident := identifier.NewDNS("example.com")
	expires := fc.Now().Add(time.Hour)

	key, _ := goodTestJWK().MarshalJSON()
	reg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key: key,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	authzIDs := createPendingAuthorization(t, sa, ident, expires)

	_, err = sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires),
			V2Authorizations: []int64{authzIDs},
			Identifiers:      []*corepb.Identifier{ident.ToProto()},
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	_, err = sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires),
			V2Authorizations: []int64{authzIDs},
			Identifiers:      []*corepb.Identifier{ident.ToProto()},
		},
	})
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")

	_, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID:      reg.Id,
		Identifiers: []*corepb.Identifier{ident.ToProto()},
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
	regA, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key: key,
	})
	test.AssertNotError(t, err, "Couldn't create test registration")

	// Add one pending authz for the first name for regA and one
	// pending authz for the second name for regA
	authzExpires := fc.Now().Add(time.Hour)
	authzIDA := createPendingAuthorization(t, sa, identifier.NewDNS("example.com"), authzExpires)
	authzIDB := createPendingAuthorization(t, sa, identifier.NewDNS("just.another.example.com"), authzExpires)

	ctx := context.Background()
	idents := identifier.ACMEIdentifiers{
		identifier.NewDNS("example.com"),
		identifier.NewDNS("just.another.example.com"),
	}

	// Call GetOrderForNames for a set of names we haven't created an order for
	// yet
	result, err := sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID:      regA.Id,
		Identifiers: idents.ToProtoSlice(),
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
			Identifiers:      idents.ToProtoSlice(),
		},
	})
	// It shouldn't error
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")
	// The order ID shouldn't be nil
	test.AssertNotNil(t, order.Id, "NewOrderAndAuthzs returned with a nil Id")

	// Call GetOrderForNames with the same account ID and set of names as the
	// above NewOrderAndAuthzs call
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID:      regA.Id,
		Identifiers: idents.ToProtoSlice(),
	})
	// It shouldn't error
	test.AssertNotError(t, err, "sa.GetOrderForNames failed")
	// The order returned should have the same ID as the order we created above
	test.AssertNotNil(t, result, "Returned order was nil")
	test.AssertEquals(t, result.Id, order.Id)

	// Call GetOrderForNames with a different account ID from the NewOrderAndAuthzs call
	regB := int64(1337)
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID:      regB,
		Identifiers: idents.ToProtoSlice(),
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
		AcctID:      regA.Id,
		Identifiers: idents.ToProtoSlice(),
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
	authzIDC := createFinalizedAuthorization(t, sa, identifier.NewDNS("zombo.com"), authzExpires, "valid", attemptedAt)
	authzIDD := createFinalizedAuthorization(t, sa, identifier.NewDNS("welcome.to.zombo.com"), authzExpires, "valid", attemptedAt)

	// Add a fresh order that uses the authorizations created above
	expires = fc.Now().Add(orderLifetime)
	order, err = sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   regA.Id,
			Expires:          timestamppb.New(expires),
			V2Authorizations: []int64{authzIDC, authzIDD},
			Identifiers:      idents.ToProtoSlice(),
		},
	})
	// It shouldn't error
	test.AssertNotError(t, err, "sa.NewOrderAndAuthzs failed")
	// The order ID shouldn't be nil
	test.AssertNotNil(t, order.Id, "NewOrderAndAuthzs returned with a nil Id")

	// Call GetOrderForNames with the same account ID and set of names as
	// the earlier NewOrderAndAuthzs call
	result, err = sa.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID:      regA.Id,
		Identifiers: idents.ToProtoSlice(),
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
		AcctID:      regA.Id,
		Identifiers: idents.ToProtoSlice(),
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
	pendingID := createPendingAuthorization(t, sa, identifier.NewDNS("pending.your.order.is.up"), expires)
	expiredID := createPendingAuthorization(t, sa, identifier.NewDNS("expired.your.order.is.up"), alreadyExpired)
	invalidID := createFinalizedAuthorization(t, sa, identifier.NewDNS("invalid.your.order.is.up"), expires, "invalid", attemptedAt)
	validID := createFinalizedAuthorization(t, sa, identifier.NewDNS("valid.your.order.is.up"), expires, "valid", attemptedAt)
	deactivatedID := createPendingAuthorization(t, sa, identifier.NewDNS("deactivated.your.order.is.up"), expires)
	_, err := sa.DeactivateAuthorization2(context.Background(), &sapb.AuthorizationID2{Id: deactivatedID})
	test.AssertNotError(t, err, "sa.DeactivateAuthorization2 failed")

	testCases := []struct {
		Name             string
		AuthorizationIDs []int64
		OrderIdents      identifier.ACMEIdentifiers
		OrderExpires     *timestamppb.Timestamp
		ExpectedStatus   string
		SetProcessing    bool
		Finalize         bool
	}{
		{
			Name: "Order with an invalid authz",
			OrderIdents: identifier.ACMEIdentifiers{
				identifier.NewDNS("pending.your.order.is.up"),
				identifier.NewDNS("invalid.your.order.is.up"),
				identifier.NewDNS("deactivated.your.order.is.up"),
				identifier.NewDNS("valid.your.order.is.up"),
			},
			AuthorizationIDs: []int64{pendingID, invalidID, deactivatedID, validID},
			ExpectedStatus:   string(core.StatusInvalid),
		},
		{
			Name: "Order with an expired authz",
			OrderIdents: identifier.ACMEIdentifiers{
				identifier.NewDNS("pending.your.order.is.up"),
				identifier.NewDNS("expired.your.order.is.up"),
				identifier.NewDNS("deactivated.your.order.is.up"),
				identifier.NewDNS("valid.your.order.is.up"),
			},
			AuthorizationIDs: []int64{pendingID, expiredID, deactivatedID, validID},
			ExpectedStatus:   string(core.StatusInvalid),
		},
		{
			Name: "Order with a deactivated authz",
			OrderIdents: identifier.ACMEIdentifiers{
				identifier.NewDNS("pending.your.order.is.up"),
				identifier.NewDNS("deactivated.your.order.is.up"),
				identifier.NewDNS("valid.your.order.is.up"),
			},
			AuthorizationIDs: []int64{pendingID, deactivatedID, validID},
			ExpectedStatus:   string(core.StatusInvalid),
		},
		{
			Name: "Order with a pending authz",
			OrderIdents: identifier.ACMEIdentifiers{
				identifier.NewDNS("valid.your.order.is.up"),
				identifier.NewDNS("pending.your.order.is.up"),
			},
			AuthorizationIDs: []int64{validID, pendingID},
			ExpectedStatus:   string(core.StatusPending),
		},
		{
			Name:             "Order with only valid authzs, not yet processed or finalized",
			OrderIdents:      identifier.ACMEIdentifiers{identifier.NewDNS("valid.your.order.is.up")},
			AuthorizationIDs: []int64{validID},
			ExpectedStatus:   string(core.StatusReady),
		},
		{
			Name:             "Order with only valid authzs, set processing",
			OrderIdents:      identifier.ACMEIdentifiers{identifier.NewDNS("valid.your.order.is.up")},
			AuthorizationIDs: []int64{validID},
			SetProcessing:    true,
			ExpectedStatus:   string(core.StatusProcessing),
		},
		{
			Name:             "Order with only valid authzs, not yet processed or finalized, OrderReadyStatus feature flag",
			OrderIdents:      identifier.ACMEIdentifiers{identifier.NewDNS("valid.your.order.is.up")},
			AuthorizationIDs: []int64{validID},
			ExpectedStatus:   string(core.StatusReady),
		},
		{
			Name:             "Order with only valid authzs, set processing",
			OrderIdents:      identifier.ACMEIdentifiers{identifier.NewDNS("valid.your.order.is.up")},
			AuthorizationIDs: []int64{validID},
			SetProcessing:    true,
			ExpectedStatus:   string(core.StatusProcessing),
		},
		{
			Name:             "Order with only valid authzs, set processing and finalized",
			OrderIdents:      identifier.ACMEIdentifiers{identifier.NewDNS("valid.your.order.is.up")},
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
					Identifiers:      tc.OrderIdents.ToProtoSlice(),
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
	authzID := createFinalizedAuthorization(t, sa, identifier.NewDNS("example.com"), expires, "valid", attemptedAt)

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

	assertIsRenewal := func(t *testing.T, issuedName string, expected bool) {
		t.Helper()
		var count int
		err := sa.dbMap.SelectOne(
			ctx,
			&count,
			`SELECT COUNT(*) FROM issuedNames
		WHERE reversedName = ?
		AND renewal = ?`,
			issuedName,
			expected,
		)
		test.AssertNotError(t, err, "Unexpected error from SelectOne on issuedNames")
		test.AssertEquals(t, count, 1)
	}

	// Add a certificate with never-before-seen identifiers.
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

	// No identifier should have an issuedNames row marking it as a renewal.
	for _, name := range testCert.DNSNames {
		assertIsRenewal(t, reverseFQDN(name), false)
	}
	for _, ip := range testCert.IPAddresses {
		assertIsRenewal(t, ip.String(), false)
	}

	// Make a new cert and add its FQDN set to the db so it will be considered a
	// renewal
	serial, testCert := test.ThrowAwayCert(t, fc)
	err = addFQDNSet(ctx, sa.dbMap, identifier.FromCert(testCert), serial, testCert.NotBefore, testCert.NotAfter)
	test.AssertNotError(t, err, "Failed to add identifier set")
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

	// Each identifier should have an issuedNames row marking it as a renewal.
	for _, name := range testCert.DNSNames {
		assertIsRenewal(t, reverseFQDN(name), true)
	}
	for _, ip := range testCert.IPAddresses {
		assertIsRenewal(t, ip.String(), true)
	}
}

func TestFinalizeAuthorization2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	fc.Set(mustTime("2021-01-01 00:00"))

	authzID := createPendingAuthorization(t, sa, identifier.NewDNS("aaa"), fc.Now().Add(time.Hour))
	expires := fc.Now().Add(time.Hour * 2).UTC()
	attemptedAt := fc.Now()
	ip, _ := netip.MustParseAddr("1.1.1.1").MarshalText()

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

	authzID = createPendingAuthorization(t, sa, identifier.NewDNS("aaa"), fc.Now().Add(time.Hour))
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

	fc.Set(mustTime("2021-01-01 00:00"))

	expires := fc.Now().Add(time.Hour * 2).UTC()
	attemptedAt := fc.Now()
	ip, _ := netip.MustParseAddr("1.1.1.1").MarshalText()

	// Implicit good port with good scheme
	authzID := createPendingAuthorization(t, sa, identifier.NewDNS("aaa"), fc.Now().Add(time.Hour))
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
	authzID = createPendingAuthorization(t, sa, identifier.NewDNS("aaa"), fc.Now().Add(time.Hour))
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
	authzID = createPendingAuthorization(t, sa, identifier.NewDNS("aaa"), fc.Now().Add(time.Hour))
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
	authzID = createPendingAuthorization(t, sa, identifier.NewDNS("aaa"), fc.Now().Add(time.Hour))
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
	authzID = createPendingAuthorization(t, sa, identifier.NewDNS("aaa"), fc.Now().Add(time.Hour))
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

func TestCountPendingAuthorizations2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	expiresA := fc.Now().Add(time.Hour).UTC()
	expiresB := fc.Now().Add(time.Hour * 3).UTC()
	_ = createPendingAuthorization(t, sa, identifier.NewDNS("example.com"), expiresA)
	_ = createPendingAuthorization(t, sa, identifier.NewDNS("example.com"), expiresB)

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
	input := map[identifier.ACMEIdentifier]authzModel{
		identifier.NewDNS("example.com"): {
			ID:              123,
			IdentifierType:  0,
			IdentifierValue: "example.com",
			RegistrationID:  77,
			Status:          1,
			Expires:         baseExpires,
			Challenges:      4,
		},
		identifier.NewDNS("www.example.com"): {
			ID:              124,
			IdentifierType:  0,
			IdentifierValue: "www.example.com",
			RegistrationID:  77,
			Status:          1,
			Expires:         baseExpires,
			Challenges:      1,
		},
		identifier.NewDNS("other.example.net"): {
			ID:              125,
			IdentifierType:  0,
			IdentifierValue: "other.example.net",
			RegistrationID:  77,
			Status:          1,
			Expires:         baseExpires,
			Challenges:      3,
		},
		identifier.NewIP(netip.MustParseAddr("10.10.10.10")): {
			ID:              126,
			IdentifierType:  1,
			IdentifierValue: "10.10.10.10",
			RegistrationID:  77,
			Status:          1,
			Expires:         baseExpires,
			Challenges:      5,
		},
	}

	out, err := authzModelMapToPB(input)
	if err != nil {
		t.Fatal(err)
	}

	for _, authzPB := range out.Authzs {
		model, ok := input[identifier.FromProto(authzPB.Identifier)]
		if !ok {
			t.Errorf("output had element for %q, an identifier not present in input", authzPB.Identifier.Value)
		}
		test.AssertEquals(t, authzPB.Id, fmt.Sprintf("%d", model.ID))
		test.AssertEquals(t, authzPB.Identifier.Type, string(uintToIdentifierType[model.IdentifierType]))
		test.AssertEquals(t, authzPB.Identifier.Value, model.IdentifierValue)
		test.AssertEquals(t, authzPB.RegistrationID, model.RegistrationID)
		test.AssertEquals(t, authzPB.Status, string(uintToStatus[model.Status]))
		gotTime := authzPB.Expires.AsTime()
		if !model.Expires.Equal(gotTime) {
			t.Errorf("Times didn't match. Got %s, expected %s (%s)", gotTime, model.Expires, authzPB.Expires.AsTime())
		}
		if len(authzPB.Challenges) != bits.OnesCount(uint(model.Challenges)) {
			t.Errorf("wrong number of challenges for %q: got %d, expected %d", authzPB.Identifier.Value,
				len(authzPB.Challenges), bits.OnesCount(uint(model.Challenges)))
		}
		switch model.Challenges {
		case 1:
			test.AssertEquals(t, authzPB.Challenges[0].Type, "http-01")
		case 3:
			test.AssertEquals(t, authzPB.Challenges[0].Type, "http-01")
			test.AssertEquals(t, authzPB.Challenges[1].Type, "dns-01")
		case 4:
			test.AssertEquals(t, authzPB.Challenges[0].Type, "tls-alpn-01")
		}

		delete(input, identifier.FromProto(authzPB.Identifier))
	}

	for k := range input {
		t.Errorf("hostname %q was not present in output", k)
	}
}

func TestGetValidOrderAuthorizations2(t *testing.T) {
	sa, fc, cleanup := initSA(t)
	defer cleanup()

	// Create three new valid authorizations
	reg := createWorkingRegistration(t, sa)
	identA := identifier.NewDNS("a.example.com")
	identB := identifier.NewDNS("b.example.com")
	identC := identifier.NewIP(netip.MustParseAddr("3fff:aaa:aaaa:aaaa:abad:0ff1:cec0:ffee"))
	expires := fc.Now().Add(time.Hour * 24 * 7).UTC()
	attemptedAt := fc.Now()

	authzIDA := createFinalizedAuthorization(t, sa, identA, expires, "valid", attemptedAt)
	authzIDB := createFinalizedAuthorization(t, sa, identB, expires, "valid", attemptedAt)
	authzIDC := createFinalizedAuthorization(t, sa, identC, expires, "valid", attemptedAt)

	orderExpr := fc.Now().Truncate(time.Second)
	order, err := sa.NewOrderAndAuthzs(context.Background(), &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID: reg.Id,
			Expires:        timestamppb.New(orderExpr),
			Identifiers: []*corepb.Identifier{
				identifier.NewDNS("a.example.com").ToProto(),
				identifier.NewDNS("b.example.com").ToProto(),
				identifier.NewIP(netip.MustParseAddr("3fff:aaa:aaaa:aaaa:abad:0ff1:cec0:ffee")).ToProto(),
			},
			V2Authorizations: []int64{authzIDA, authzIDB, authzIDC},
		},
	})
	test.AssertNotError(t, err, "AddOrder failed")

	authzPBs, err := sa.GetValidOrderAuthorizations2(
		context.Background(),
		&sapb.GetValidOrderAuthorizationsRequest{
			Id:     order.Id,
			AcctID: reg.Id,
		})
	test.AssertNotError(t, err, "sa.GetValidOrderAuthorizations failed")
	test.AssertNotNil(t, authzPBs, "sa.GetValidOrderAuthorizations result was nil")
	test.AssertEquals(t, len(authzPBs.Authzs), 3)

	identsToCheck := map[identifier.ACMEIdentifier]int64{
		identifier.NewDNS("a.example.com"):                                              authzIDA,
		identifier.NewDNS("b.example.com"):                                              authzIDB,
		identifier.NewIP(netip.MustParseAddr("3fff:aaa:aaaa:aaaa:abad:0ff1:cec0:ffee")): authzIDC,
	}
	for _, a := range authzPBs.Authzs {
		ident := identifier.ACMEIdentifier{Type: identifier.IdentifierType(a.Identifier.Type), Value: a.Identifier.Value}
		if fmt.Sprintf("%d", identsToCheck[ident]) != a.Id {
			t.Fatalf("incorrect identifier %q with id %s", a.Identifier.Value, a.Id)
		}
		test.AssertEquals(t, a.Expires.AsTime(), expires)
		delete(identsToCheck, ident)
	}

	// Getting the order authorizations for an order that doesn't exist should return nothing
	missingID := int64(0xC0FFEEEEEEE)
	authzPBs, err = sa.GetValidOrderAuthorizations2(
		context.Background(),
		&sapb.GetValidOrderAuthorizationsRequest{
			Id:     missingID,
			AcctID: reg.Id,
		})
	test.AssertNotError(t, err, "sa.GetValidOrderAuthorizations failed")
	test.AssertEquals(t, len(authzPBs.Authzs), 0)
}

func TestCountInvalidAuthorizations2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	fc.Add(time.Hour)
	reg := createWorkingRegistration(t, sa)
	idents := identifier.ACMEIdentifiers{
		identifier.NewDNS("aaa"),
		identifier.NewIP(netip.MustParseAddr("10.10.10.10")),
	}
	for _, ident := range idents {
		// Create two authorizations, one pending, one invalid
		expiresA := fc.Now().Add(time.Hour).UTC()
		expiresB := fc.Now().Add(time.Hour * 3).UTC()
		attemptedAt := fc.Now()
		_ = createFinalizedAuthorization(t, sa, ident, expiresA, "invalid", attemptedAt)
		_ = createPendingAuthorization(t, sa, ident, expiresB)

		earliest := fc.Now().Add(-time.Hour).UTC()
		latest := fc.Now().Add(time.Hour * 5).UTC()
		count, err := sa.CountInvalidAuthorizations2(context.Background(), &sapb.CountInvalidAuthorizationsRequest{
			RegistrationID: reg.Id,
			Identifier:     ident.ToProto(),
			Range: &sapb.Range{
				Earliest: timestamppb.New(earliest),
				Latest:   timestamppb.New(latest),
			},
		})
		test.AssertNotError(t, err, "sa.CountInvalidAuthorizations2 failed")
		test.AssertEquals(t, count.Count, int64(1))
	}
}

func TestGetValidAuthorizations2(t *testing.T) {
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

	var aaa int64
	{
		tokenStr := core.NewToken()
		token, err := base64.RawURLEncoding.DecodeString(tokenStr)
		test.AssertNotError(t, err, "computing test authorization challenge token")

		profile := "test"
		attempted := challTypeToUint[string(core.ChallengeTypeHTTP01)]
		attemptedAt := fc.Now()
		vr, _ := json.Marshal([]core.ValidationRecord{})

		am := authzModel{
			IdentifierType:         identifierTypeToUint[string(identifier.TypeDNS)],
			IdentifierValue:        "aaa",
			RegistrationID:         1,
			CertificateProfileName: &profile,
			Status:                 statusToUint[core.StatusValid],
			Expires:                fc.Now().Add(24 * time.Hour),
			Challenges:             1 << challTypeToUint[string(core.ChallengeTypeHTTP01)],
			Attempted:              &attempted,
			AttemptedAt:            &attemptedAt,
			Token:                  token,
			ValidationError:        nil,
			ValidationRecord:       vr,
		}

		err = sa.dbMap.Insert(context.Background(), &am)
		test.AssertNotError(t, err, "failed to insert valid authz")

		aaa = am.ID
	}

	for _, tc := range []struct {
		name        string
		regID       int64
		identifiers []*corepb.Identifier
		profile     string
		validUntil  time.Time
		wantIDs     []int64
	}{
		{
			name:        "happy path, DNS identifier",
			regID:       1,
			identifiers: []*corepb.Identifier{identifier.NewDNS("aaa").ToProto()},
			profile:     "test",
			validUntil:  fc.Now().Add(time.Hour),
			wantIDs:     []int64{aaa},
		},
		{
			name:        "different identifier type",
			regID:       1,
			identifiers: []*corepb.Identifier{identifier.NewIP(netip.MustParseAddr("10.10.10.10")).ToProto()},
			profile:     "test",
			validUntil:  fc.Now().Add(time.Hour),
			wantIDs:     []int64{},
		},
		{
			name:        "different regID",
			regID:       2,
			identifiers: []*corepb.Identifier{identifier.NewDNS("aaa").ToProto()},
			profile:     "test",
			validUntil:  fc.Now().Add(time.Hour),
			wantIDs:     []int64{},
		},
		{
			name:        "different DNS identifier",
			regID:       1,
			identifiers: []*corepb.Identifier{identifier.NewDNS("bbb").ToProto()},
			profile:     "test",
			validUntil:  fc.Now().Add(time.Hour),
			wantIDs:     []int64{},
		},
		{
			name:        "different profile",
			regID:       1,
			identifiers: []*corepb.Identifier{identifier.NewDNS("aaa").ToProto()},
			profile:     "other",
			validUntil:  fc.Now().Add(time.Hour),
			wantIDs:     []int64{},
		},
		{
			name:        "too-far-out validUntil",
			regID:       2,
			identifiers: []*corepb.Identifier{identifier.NewDNS("aaa").ToProto()},
			profile:     "test",
			validUntil:  fc.Now().Add(25 * time.Hour),
			wantIDs:     []int64{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sa.GetValidAuthorizations2(context.Background(), &sapb.GetValidAuthorizationsRequest{
				RegistrationID: tc.regID,
				Identifiers:    tc.identifiers,
				Profile:        tc.profile,
				ValidUntil:     timestamppb.New(tc.validUntil),
			})
			if err != nil {
				t.Fatalf("GetValidAuthorizations2 got error %q, want success", err)
			}

			var gotIDs []int64
			for _, authz := range got.Authzs {
				id, err := strconv.Atoi(authz.Id)
				if err != nil {
					t.Fatalf("parsing authz id: %s", err)
				}
				gotIDs = append(gotIDs, int64(id))
			}

			slices.Sort(gotIDs)
			slices.Sort(tc.wantIDs)
			if !slices.Equal(gotIDs, tc.wantIDs) {
				t.Errorf("GetValidAuthorizations2() = %+v, want %+v", gotIDs, tc.wantIDs)
			}
		})
	}
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
			Identifiers:      []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
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
		randInt := func() int64 { return mrand.Int64() }
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

	// The basic request covers a time range that should include this certificate.
	basicRequest := &sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  mustTimestamp("2023-03-01 00:00"),
		ExpiresBefore: mustTimestamp("2023-04-01 00:00"),
		RevokedBefore: mustTimestamp("2023-04-01 00:00"),
	}
	count, err := countRevokedCerts(basicRequest)
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Revoke the certificate.
	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   core.SerialToString(eeCert.SerialNumber),
		Date:     mustTimestamp("2023-01-01 00:00"),
		Reason:   1,
		Response: []byte{1, 2, 3},
	})
	test.AssertNotError(t, err, "failed to revoke test cert")

	// Asking for revoked certs now should return one result.
	count, err = countRevokedCerts(basicRequest)
	test.AssertNotError(t, err, "normal usage shouldn't result in error")
	test.AssertEquals(t, count, 1)

	// Asking for revoked certs with an old RevokedBefore should return no results.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  basicRequest.ExpiresAfter,
		ExpiresBefore: basicRequest.ExpiresBefore,
		RevokedBefore: mustTimestamp("2020-03-01 00:00"),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs in a time period that does not cover this cert's
	// notAfter timestamp should return zero results.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  1,
		ExpiresAfter:  mustTimestamp("2022-03-01 00:00"),
		ExpiresBefore: mustTimestamp("2022-04-01 00:00"),
		RevokedBefore: mustTimestamp("2023-04-01 00:00"),
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs from a different issuer should return zero results.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsRequest{
		IssuerNameID:  5678,
		ExpiresAfter:  basicRequest.ExpiresAfter,
		ExpiresBefore: basicRequest.ExpiresBefore,
		RevokedBefore: basicRequest.RevokedBefore,
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)
}

func TestGetRevokedCertsByShard(t *testing.T) {
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

	// Here's a little helper func we'll use to call GetRevokedCertsByShard and count
	// how many results it returned.
	countRevokedCerts := func(req *sapb.GetRevokedCertsByShardRequest) (int, error) {
		stream := make(chan *corepb.CRLEntry)
		mockServerStream := &fakeServerStream[corepb.CRLEntry]{output: stream}
		var err error
		go func() {
			err = sa.GetRevokedCertsByShard(req, mockServerStream)
			close(stream)
		}()
		entriesReceived := 0
		for range stream {
			entriesReceived++
		}
		return entriesReceived, err
	}

	// The basic request covers a time range and shard that should include this certificate.
	basicRequest := &sapb.GetRevokedCertsByShardRequest{
		IssuerNameID:  1,
		ShardIdx:      9,
		ExpiresAfter:  mustTimestamp("2023-03-01 00:00"),
		RevokedBefore: mustTimestamp("2023-04-01 00:00"),
	}

	// Nothing's been revoked yet. Count should be zero.
	count, err := countRevokedCerts(basicRequest)
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Revoke the certificate, providing the ShardIdx so it gets written into
	// both the certificateStatus and revokedCertificates tables.
	_, err = sa.RevokeCertificate(context.Background(), &sapb.RevokeCertificateRequest{
		IssuerID: 1,
		Serial:   core.SerialToString(eeCert.SerialNumber),
		Date:     mustTimestamp("2023-01-01 00:00"),
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
	count, err = countRevokedCerts(basicRequest)
	test.AssertNotError(t, err, "normal usage shouldn't result in error")
	test.AssertEquals(t, count, 1)

	// Asking for revoked certs from a different issuer should return zero results.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsByShardRequest{
		IssuerNameID:  5678,
		ShardIdx:      basicRequest.ShardIdx,
		ExpiresAfter:  basicRequest.ExpiresAfter,
		RevokedBefore: basicRequest.RevokedBefore,
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs from a different shard should return zero results.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsByShardRequest{
		IssuerNameID:  basicRequest.IssuerNameID,
		ShardIdx:      8,
		ExpiresAfter:  basicRequest.ExpiresAfter,
		RevokedBefore: basicRequest.RevokedBefore,
	})
	test.AssertNotError(t, err, "zero rows shouldn't result in error")
	test.AssertEquals(t, count, 0)

	// Asking for revoked certs with an old RevokedBefore should return no results.
	count, err = countRevokedCerts(&sapb.GetRevokedCertsByShardRequest{
		IssuerNameID:  basicRequest.IssuerNameID,
		ShardIdx:      basicRequest.ShardIdx,
		ExpiresAfter:  basicRequest.ExpiresAfter,
		RevokedBefore: mustTimestamp("2020-03-01 00:00"),
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
	test.Assert(t, crlModel.ThisUpdate.Equal(thisUpdate), "checking updated thisUpdate timestamp")

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
	sa, fc, cleanUp := initSA(t)
	defer cleanUp()

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
	authzID := createFinalizedAuthorization(t, sa, identifier.NewDNS("example.com"), expires, "valid", attemptedAt)

	// Add a new order in pending status with no certificate serial.
	expires1Year := sa.clk.Now().Add(365 * 24 * time.Hour)
	order, err := sa.NewOrderAndAuthzs(ctx, &sapb.NewOrderAndAuthzsRequest{
		NewOrder: &sapb.NewOrderRequest{
			RegistrationID:   reg.Id,
			Expires:          timestamppb.New(expires1Year),
			Identifiers:      []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
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
			Identifiers:      []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
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
			Identifiers:      []*corepb.Identifier{identifier.NewDNS("example.com").ToProto()},
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
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
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
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.net",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
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

func bulkInsertPausedIdentifiers(ctx context.Context, sa *SQLStorageAuthority, count int) error {
	const batchSize = 1000

	values := make([]interface{}, 0, batchSize*4)
	now := sa.clk.Now().Add(-time.Hour)
	batches := (count + batchSize - 1) / batchSize

	for batch := 0; batch < batches; batch++ {
		query := `
		INSERT INTO paused (registrationID, identifierType, identifierValue, pausedAt)
		VALUES`

		start := batch * batchSize
		end := start + batchSize
		if end > count {
			end = count
		}

		for i := start; i < end; i++ {
			if i > start {
				query += ","
			}
			query += "(?, ?, ?, ?)"
			values = append(values, 1, identifierTypeToUint[string(identifier.TypeDNS)], fmt.Sprintf("example%d.com", i), now)
		}

		_, err := sa.dbMap.ExecContext(ctx, query, values...)
		if err != nil {
			return fmt.Errorf("bulk inserting paused identifiers: %w", err)
		}
		values = values[:0]
	}

	return nil
}

func TestUnpauseAccountWithTwoLoops(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	err := bulkInsertPausedIdentifiers(ctx, sa, 12000)
	test.AssertNotError(t, err, "bulk inserting paused identifiers")

	result, err := sa.UnpauseAccount(ctx, &sapb.RegistrationID{Id: 1})
	test.AssertNotError(t, err, "Unexpected error for UnpauseAccount()")
	test.AssertEquals(t, result.Count, int64(12000))
}

func TestUnpauseAccountWithMaxLoops(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	err := bulkInsertPausedIdentifiers(ctx, sa, 50001)
	test.AssertNotError(t, err, "bulk inserting paused identifiers")

	result, err := sa.UnpauseAccount(ctx, &sapb.RegistrationID{Id: 1})
	test.AssertNotError(t, err, "Unexpected error for UnpauseAccount()")
	test.AssertEquals(t, result.Count, int64(50000))
}

func TestPauseIdentifiers(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	ptrTime := func(t time.Time) *time.Time {
		return &t
	}

	fourWeeksAgo := sa.clk.Now().Add(-4 * 7 * 24 * time.Hour)
	threeWeeksAgo := sa.clk.Now().Add(-3 * 7 * 24 * time.Hour)

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
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
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
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt:   fourWeeksAgo,
					UnpausedAt: ptrTime(threeWeeksAgo),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
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
			name: "One unpaused entry which was previously paused and unpaused less than 2 weeks ago",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt:   fourWeeksAgo,
					UnpausedAt: ptrTime(sa.clk.Now().Add(-13 * 24 * time.Hour)),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
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
			name: "An identifier which is currently paused",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt: fourWeeksAgo,
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
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
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt:   fourWeeksAgo,
					UnpausedAt: ptrTime(threeWeeksAgo),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.net",
					},
					PausedAt:   fourWeeksAgo,
					UnpausedAt: ptrTime(threeWeeksAgo),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.net",
					},
					{
						Type:  string(identifier.TypeDNS),
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
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.com",
					},
				},
			},
			want: &sapb.Identifiers{
				Identifiers: []*corepb.Identifier{},
			},
		},
		{
			name: "One paused identifier",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.com",
					},
				},
			},
			want: &sapb.Identifiers{
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
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
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.net",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.org",
					},
					PausedAt:   sa.clk.Now().Add(-time.Hour),
					UnpausedAt: ptrTime(sa.clk.Now().Add(-time.Minute)),
				},
			},
			req: &sapb.PauseRequest{
				RegistrationID: 1,
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.net",
					},
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.org",
					},
				},
			},
			want: &sapb.Identifiers{
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.TypeDNS),
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
				Identifiers: []*corepb.Identifier{},
			},
		},
		{
			name: "One paused identifier",
			state: []pausedModel{
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
			},
			req: &sapb.RegistrationID{Id: 1},
			want: &sapb.Identifiers{
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
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
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.com",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.net",
					},
					PausedAt: sa.clk.Now().Add(-time.Hour),
				},
				{
					RegistrationID: 1,
					identifierModel: identifierModel{
						Type:  identifierTypeToUint[string(identifier.TypeDNS)],
						Value: "example.org",
					},
					PausedAt:   sa.clk.Now().Add(-time.Hour),
					UnpausedAt: ptrTime(sa.clk.Now().Add(-time.Minute)),
				},
			},
			req: &sapb.RegistrationID{Id: 1},
			want: &sapb.Identifiers{
				Identifiers: []*corepb.Identifier{
					{
						Type:  string(identifier.TypeDNS),
						Value: "example.com",
					},
					{
						Type:  string(identifier.TypeDNS),
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
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	// Insert two paused identifiers for two different accounts.
	err := sa.dbMap.Insert(ctx, &pausedModel{
		RegistrationID: 1,
		identifierModel: identifierModel{
			Type:  identifierTypeToUint[string(identifier.TypeDNS)],
			Value: "example.com",
		},
		PausedAt: sa.clk.Now().Add(-time.Hour),
	})
	test.AssertNotError(t, err, "inserting test identifier")

	err = sa.dbMap.Insert(ctx, &pausedModel{
		RegistrationID: 2,
		identifierModel: identifierModel{
			Type:  identifierTypeToUint[string(identifier.TypeDNS)],
			Value: "example.net",
		},
		PausedAt: sa.clk.Now().Add(-time.Hour),
	})
	test.AssertNotError(t, err, "inserting test identifier")

	// Unpause the first account.
	_, err = sa.UnpauseAccount(ctx, &sapb.RegistrationID{Id: 1})
	test.AssertNotError(t, err, "UnpauseAccount failed")

	// Check that the second account's identifier is still paused.
	idents, err := sa.GetPausedIdentifiers(ctx, &sapb.RegistrationID{Id: 2})
	test.AssertNotError(t, err, "GetPausedIdentifiers failed")
	test.AssertEquals(t, len(idents.Identifiers), 1)
	test.AssertEquals(t, idents.Identifiers[0].Value, "example.net")
}

func newAcctKey(t *testing.T) []byte {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	jwk := &jose.JSONWebKey{Key: key.Public()}
	acctKey, err := jwk.MarshalJSON()
	test.AssertNotError(t, err, "failed to marshal account key")
	return acctKey
}

func TestUpdateRegistrationContact(t *testing.T) {
	// TODO(#8199): Delete this.
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	noContact, _ := json.Marshal("")
	exampleContact, _ := json.Marshal("test@example.com")
	twoExampleContacts, _ := json.Marshal([]string{"test1@example.com", "test2@example.com"})

	_, err := sa.UpdateRegistrationContact(ctx, &sapb.UpdateRegistrationContactRequest{})
	test.AssertError(t, err, "should not have been able to update registration contact without a registration ID")
	test.AssertContains(t, err.Error(), "incomplete gRPC request message")

	tests := []struct {
		name            string
		oldContactsJSON []string
		newContacts     []string
	}{
		{
			name:            "update a valid registration from no contacts to one email address",
			oldContactsJSON: []string{string(noContact)},
			newContacts:     []string{"mailto:test@example.com"},
		},
		{
			name:            "update a valid registration from no contacts to two email addresses",
			oldContactsJSON: []string{string(noContact)},
			newContacts:     []string{"mailto:test1@example.com", "mailto:test2@example.com"},
		},
		{
			name:            "update a valid registration from one email address to no contacts",
			oldContactsJSON: []string{string(exampleContact)},
			newContacts:     []string{},
		},
		{
			name:            "update a valid registration from one email address to two email addresses",
			oldContactsJSON: []string{string(exampleContact)},
			newContacts:     []string{"mailto:test1@example.com", "mailto:test2@example.com"},
		},
		{
			name:            "update a valid registration from two email addresses to no contacts",
			oldContactsJSON: []string{string(twoExampleContacts)},
			newContacts:     []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, err := sa.NewRegistration(ctx, &corepb.Registration{
				Contact: tt.oldContactsJSON,
				Key:     newAcctKey(t),
			})
			test.AssertNotError(t, err, "creating new registration")

			updatedReg, err := sa.UpdateRegistrationContact(ctx, &sapb.UpdateRegistrationContactRequest{
				RegistrationID: reg.Id,
				Contacts:       tt.newContacts,
			})
			test.AssertNotError(t, err, "unexpected error for UpdateRegistrationContact()")
			test.AssertEquals(t, updatedReg.Id, reg.Id)
			test.AssertEquals(t, len(updatedReg.Contact), 0)

			refetchedReg, err := sa.GetRegistration(ctx, &sapb.RegistrationID{Id: reg.Id})
			test.AssertNotError(t, err, "retrieving registration")
			test.AssertEquals(t, refetchedReg.Id, reg.Id)
			test.AssertEquals(t, len(refetchedReg.Contact), 0)
		})
	}
}

func TestUpdateRegistrationKey(t *testing.T) {
	sa, _, cleanUp := initSA(t)
	defer cleanUp()

	_, err := sa.UpdateRegistrationKey(ctx, &sapb.UpdateRegistrationKeyRequest{})
	test.AssertError(t, err, "should not have been able to update registration key without a registration ID")
	test.AssertContains(t, err.Error(), "incomplete gRPC request message")

	existingReg, err := sa.NewRegistration(ctx, &corepb.Registration{
		Key: newAcctKey(t),
	})
	test.AssertNotError(t, err, "creating new registration")

	tests := []struct {
		name          string
		newJwk        []byte
		expectedError string
	}{
		{
			name:   "update a valid registration with a new account key",
			newJwk: newAcctKey(t),
		},
		{
			name:          "update a valid registration with a duplicate account key",
			newJwk:        existingReg.Key,
			expectedError: "key is already in use for a different account",
		},
		{
			name:          "update a valid registration with a malformed account key",
			newJwk:        []byte("Eat at Joe's"),
			expectedError: "parsing JWK",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, err := sa.NewRegistration(ctx, &corepb.Registration{
				Key: newAcctKey(t),
			})
			test.AssertNotError(t, err, "creating new registration")

			updatedReg, err := sa.UpdateRegistrationKey(ctx, &sapb.UpdateRegistrationKeyRequest{
				RegistrationID: reg.Id,
				Jwk:            tt.newJwk,
			})
			if tt.expectedError != "" {
				test.AssertError(t, err, "should have errored")
				test.AssertContains(t, err.Error(), tt.expectedError)
			} else {
				test.AssertNotError(t, err, "unexpected error for UpdateRegistrationKey()")
				test.AssertEquals(t, updatedReg.Id, reg.Id)
				test.AssertDeepEquals(t, updatedReg.Key, tt.newJwk)

				refetchedReg, err := sa.GetRegistration(ctx, &sapb.RegistrationID{
					Id: reg.Id,
				})
				test.AssertNotError(t, err, "retrieving registration")
				test.AssertDeepEquals(t, refetchedReg.Key, tt.newJwk)
			}
		})
	}
}

type mockRLOStream struct {
	grpc.ServerStream
	sent []*sapb.RateLimitOverride
	ctx  context.Context
}

func newMockRLOStream() *mockRLOStream {
	return &mockRLOStream{ctx: ctx}
}
func (m *mockRLOStream) Context() context.Context { return m.ctx }
func (m *mockRLOStream) RecvMsg(any) error        { return io.EOF }
func (m *mockRLOStream) Send(ov *sapb.RateLimitOverride) error {
	m.sent = append(m.sent, ov)
	return nil
}

func TestAddRateLimitOverrideInsertThenUpdate(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		// TODO(#8147): Remove this skip.
		t.Skip("skipping, this overrides table must exist for this test to run")
	}

	sa, _, cleanup := initSA(t)
	defer cleanup()

	expectBucketKey := core.RandomString(10)
	ov := &sapb.RateLimitOverride{
		LimitEnum: 1,
		BucketKey: expectBucketKey,
		Comment:   "insert",
		Period:    durationpb.New(time.Hour),
		Count:     100,
		Burst:     100,
	}

	// Insert
	resp, err := sa.AddRateLimitOverride(ctx, &sapb.AddRateLimitOverrideRequest{Override: ov})
	test.AssertNotError(t, err, "expected successful insert, got error")
	test.Assert(t, resp.Inserted && resp.Enabled, fmt.Sprintf("expected (Inserted=true, Enabled=true) for initial insert, got (%v,%v)", resp.Inserted, resp.Enabled))

	// Update (change comment)
	ov.Comment = "updated"
	resp, err = sa.AddRateLimitOverride(ctx, &sapb.AddRateLimitOverrideRequest{Override: ov})
	test.AssertNotError(t, err, "expected successful update, got error")
	test.Assert(t, !resp.Inserted && resp.Enabled, fmt.Sprintf("expected (Inserted=false, Enabled=true) for update, got (%v, %v)", resp.Inserted, resp.Enabled))

	got, err := sa.GetRateLimitOverride(ctx, &sapb.GetRateLimitOverrideRequest{LimitEnum: 1, BucketKey: expectBucketKey})
	test.AssertNotError(t, err, "expected GetRateLimitOverride to succeed, got error")
	test.AssertEquals(t, got.Override.Comment, "updated")

	// Disable
	_, err = sa.DisableRateLimitOverride(ctx, &sapb.DisableRateLimitOverrideRequest{LimitEnum: 1, BucketKey: expectBucketKey})
	test.AssertNotError(t, err, "expected DisableRateLimitOverride to succeed, got error")

	// Update and check that it's still disabled.
	got, err = sa.GetRateLimitOverride(ctx, &sapb.GetRateLimitOverrideRequest{LimitEnum: 1, BucketKey: expectBucketKey})
	test.AssertNotError(t, err, "expected GetRateLimitOverride to succeed, got error")
	test.Assert(t, !got.Enabled, fmt.Sprintf("expected Enabled=false after disable, got Enabled=%v", got.Enabled))

	// Update (change period, count, and burst)
	ov.Period = durationpb.New(2 * time.Hour)
	ov.Count = 200
	ov.Burst = 200
	_, err = sa.AddRateLimitOverride(ctx, &sapb.AddRateLimitOverrideRequest{Override: ov})
	test.AssertNotError(t, err, "expected successful update, got error")

	got, err = sa.GetRateLimitOverride(ctx, &sapb.GetRateLimitOverrideRequest{LimitEnum: 1, BucketKey: expectBucketKey})
	test.AssertNotError(t, err, "expected GetRateLimitOverride to succeed, got error")
	test.AssertEquals(t, got.Override.Period.AsDuration(), 2*time.Hour)
	test.AssertEquals(t, got.Override.Count, int64(200))
	test.AssertEquals(t, got.Override.Burst, int64(200))
}

func TestDisableEnableRateLimitOverride(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		// TODO(#8147): Remove this skip.
		t.Skip("skipping, this overrides table must exist for this test to run")
	}

	sa, _, cleanup := initSA(t)
	defer cleanup()

	expectBucketKey := core.RandomString(10)
	ov := &sapb.RateLimitOverride{
		LimitEnum: 2,
		BucketKey: expectBucketKey,
		Period:    durationpb.New(time.Hour),
		Count:     1,
		Burst:     1,
		Comment:   "test",
	}
	_, _ = sa.AddRateLimitOverride(ctx, &sapb.AddRateLimitOverrideRequest{Override: ov})

	// Disable
	_, err := sa.DisableRateLimitOverride(ctx,
		&sapb.DisableRateLimitOverrideRequest{LimitEnum: 2, BucketKey: expectBucketKey})
	test.AssertNotError(t, err, "expected DisableRateLimitOverride to succeed, got error")

	st, _ := sa.GetRateLimitOverride(ctx,
		&sapb.GetRateLimitOverrideRequest{LimitEnum: 2, BucketKey: expectBucketKey})
	test.Assert(t, !st.Enabled,
		fmt.Sprintf("expected Enabled=false after disable, got Enabled=%v", st.Enabled))

	// Enable
	_, err = sa.EnableRateLimitOverride(ctx,
		&sapb.EnableRateLimitOverrideRequest{LimitEnum: 2, BucketKey: expectBucketKey})
	test.AssertNotError(t, err, "expected EnableRateLimitOverride to succeed, got error")

	st, _ = sa.GetRateLimitOverride(ctx,
		&sapb.GetRateLimitOverrideRequest{LimitEnum: 2, BucketKey: expectBucketKey})
	test.Assert(t, st.Enabled,
		fmt.Sprintf("expected Enabled=true after enable, got Enabled=%v", st.Enabled))
}

func TestGetEnabledRateLimitOverrides(t *testing.T) {
	if os.Getenv("BOULDER_CONFIG_DIR") != "test/config-next" {
		// TODO(#8147): Remove this skip.
		t.Skip("skipping, this overrides table must exist for this test to run")
	}

	sa, _, cleanup := initSA(t)
	defer cleanup()

	// Enabled
	ov1 := &sapb.RateLimitOverride{
		LimitEnum: 10, BucketKey: "on", Period: durationpb.New(time.Second), Count: 1, Burst: 1, Comment: "on",
	}
	// Disabled
	ov2 := &sapb.RateLimitOverride{
		LimitEnum: 11, BucketKey: "off", Period: durationpb.New(time.Second), Count: 1, Burst: 1, Comment: "off",
	}

	_, err := sa.AddRateLimitOverride(ctx, &sapb.AddRateLimitOverrideRequest{Override: ov1})
	test.AssertNotError(t, err, "expected successful insert of ov1, got error")
	_, err = sa.AddRateLimitOverride(ctx, &sapb.AddRateLimitOverrideRequest{Override: ov2})
	test.AssertNotError(t, err, "expected successful insert of ov2, got error")
	_, err = sa.DisableRateLimitOverride(ctx, &sapb.DisableRateLimitOverrideRequest{LimitEnum: 11, BucketKey: "off"})
	test.AssertNotError(t, err, "expected DisableRateLimitOverride of ov2 to succeed, got error")
	_, err = sa.EnableRateLimitOverride(ctx, &sapb.EnableRateLimitOverrideRequest{LimitEnum: 10, BucketKey: "on"})
	test.AssertNotError(t, err, "expected EnableRateLimitOverride of ov1 to succeed, got error")

	stream := newMockRLOStream()
	err = sa.GetEnabledRateLimitOverrides(&emptypb.Empty{}, stream)
	test.AssertNotError(t, err, "expected streaming enabled overrides to succeed, got error")
	test.AssertEquals(t, len(stream.sent), 1)
	test.AssertEquals(t, stream.sent[0].BucketKey, "on")
}
