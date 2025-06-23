package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	berrors "github.com/letsencrypt/boulder/errors"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/mocks"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
)

// mockSAWithIncident is a mock which only implements the SerialsForIncident
// gRPC method. It can be initialized with a set of serials for that method
// to return.
type mockSAWithIncident struct {
	sapb.StorageAuthorityReadOnlyClient
	incidentSerials []string
}

// SerialsForIncident returns a fake gRPC stream client object which itself
// will return the mockSAWithIncident's serials in order.
func (msa *mockSAWithIncident) SerialsForIncident(_ context.Context, _ *sapb.SerialsForIncidentRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[sapb.IncidentSerial], error) {
	fakeResults := make([]*sapb.IncidentSerial, len(msa.incidentSerials))
	for i, serial := range msa.incidentSerials {
		fakeResults[i] = &sapb.IncidentSerial{Serial: serial}
	}
	return &mocks.ServerStreamClient[sapb.IncidentSerial]{Results: fakeResults}, nil
}

func TestSerialsFromIncidentTable(t *testing.T) {
	t.Parallel()
	serials := []string{"foo", "bar", "baz"}

	a := admin{
		saroc: &mockSAWithIncident{incidentSerials: serials},
	}

	res, err := a.serialsFromIncidentTable(context.Background(), "tablename")
	test.AssertNotError(t, err, "getting serials from mock SA")
	test.AssertDeepEquals(t, res, serials)
}

func TestSerialsFromFile(t *testing.T) {
	t.Parallel()
	serials := []string{"foo", "bar", "baz"}

	serialsFile := path.Join(t.TempDir(), "serials.txt")
	err := os.WriteFile(serialsFile, []byte(strings.Join(serials, "\n")), os.ModeAppend)
	test.AssertNotError(t, err, "writing temp serials file")

	a := admin{}

	res, err := a.serialsFromFile(context.Background(), serialsFile)
	test.AssertNotError(t, err, "getting serials from file")
	test.AssertDeepEquals(t, res, serials)
}

// mockSAWithKey is a mock which only implements the GetSerialsByKey
// gRPC method. It can be initialized with a set of serials for that method
// to return.
type mockSAWithKey struct {
	sapb.StorageAuthorityReadOnlyClient
	keyHash []byte
	serials []string
}

// GetSerialsByKey returns a fake gRPC stream client object which itself
// will return the mockSAWithKey's serials in order.
func (msa *mockSAWithKey) GetSerialsByKey(_ context.Context, req *sapb.SPKIHash, _ ...grpc.CallOption) (grpc.ServerStreamingClient[sapb.Serial], error) {
	if !slices.Equal(req.KeyHash, msa.keyHash) {
		return &mocks.ServerStreamClient[sapb.Serial]{}, nil
	}
	fakeResults := make([]*sapb.Serial, len(msa.serials))
	for i, serial := range msa.serials {
		fakeResults[i] = &sapb.Serial{Serial: serial}
	}
	return &mocks.ServerStreamClient[sapb.Serial]{Results: fakeResults}, nil
}

func TestSerialsFromPrivateKey(t *testing.T) {
	serials := []string{"foo", "bar", "baz"}
	fc := clock.NewFake()
	fc.Set(time.Now())

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating test private key")
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	test.AssertNotError(t, err, "marshalling test private key bytes")

	keyFile := path.Join(t.TempDir(), "key.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})
	err = os.WriteFile(keyFile, keyPEM, os.ModeAppend)
	test.AssertNotError(t, err, "writing test private key file")

	keyHash, err := core.KeyDigest(privKey.Public())
	test.AssertNotError(t, err, "computing test SPKI hash")

	a := admin{saroc: &mockSAWithKey{keyHash: keyHash[:], serials: serials}}

	res, err := a.serialsFromPrivateKey(context.Background(), keyFile)
	test.AssertNotError(t, err, "getting serials from keyHashToSerial table")
	test.AssertDeepEquals(t, res, serials)
}

// mockSAWithAccount is a mock which only implements the GetSerialsByAccount
// gRPC method. It can be initialized with a set of serials for that method
// to return.
type mockSAWithAccount struct {
	sapb.StorageAuthorityReadOnlyClient
	regID   int64
	serials []string
}

func (msa *mockSAWithAccount) GetRegistration(_ context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (*corepb.Registration, error) {
	if req.Id != msa.regID {
		return nil, errors.New("no such reg")
	}
	return &corepb.Registration{}, nil
}

// GetSerialsByAccount returns a fake gRPC stream client object which itself
// will return the mockSAWithAccount's serials in order.
func (msa *mockSAWithAccount) GetSerialsByAccount(_ context.Context, req *sapb.RegistrationID, _ ...grpc.CallOption) (grpc.ServerStreamingClient[sapb.Serial], error) {
	if req.Id != msa.regID {
		return &mocks.ServerStreamClient[sapb.Serial]{}, nil
	}
	fakeResults := make([]*sapb.Serial, len(msa.serials))
	for i, serial := range msa.serials {
		fakeResults[i] = &sapb.Serial{Serial: serial}
	}
	return &mocks.ServerStreamClient[sapb.Serial]{Results: fakeResults}, nil
}

func TestSerialsFromRegID(t *testing.T) {
	serials := []string{"foo", "bar", "baz"}
	a := admin{saroc: &mockSAWithAccount{regID: 123, serials: serials}}

	res, err := a.serialsFromRegID(context.Background(), 123)
	test.AssertNotError(t, err, "getting serials from serials table")
	test.AssertDeepEquals(t, res, serials)
}

// mockRARecordingRevocations is a mock which only implements the
// AdministrativelyRevokeCertificate gRPC method. It can be initialized with
// serials to recognize as already revoked, or to fail.
type mockRARecordingRevocations struct {
	rapb.RegistrationAuthorityClient
	doomedToFail       []string
	alreadyRevoked     []string
	revocationRequests []*rapb.AdministrativelyRevokeCertificateRequest
	sync.Mutex
}

// AdministrativelyRevokeCertificate records the request it received on the mock
// RA struct, and succeeds if it doesn't recognize the serial as one it should
// fail for.
func (mra *mockRARecordingRevocations) AdministrativelyRevokeCertificate(_ context.Context, req *rapb.AdministrativelyRevokeCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	mra.Lock()
	defer mra.Unlock()
	mra.revocationRequests = append(mra.revocationRequests, req)
	if slices.Contains(mra.doomedToFail, req.Serial) {
		return nil, errors.New("oops")
	}
	if slices.Contains(mra.alreadyRevoked, req.Serial) {
		return nil, berrors.AlreadyRevokedError("too slow")
	}
	return &emptypb.Empty{}, nil
}

func (mra *mockRARecordingRevocations) reset() {
	mra.doomedToFail = nil
	mra.alreadyRevoked = nil
	mra.revocationRequests = nil
}

func TestRevokeSerials(t *testing.T) {
	t.Parallel()
	serials := []string{
		"2a:18:59:2b:7f:4b:f5:96:fb:1a:1d:f1:35:56:7a:cd:82:5a",
		"03:8c:3f:63:88:af:b7:69:5d:d4:d6:bb:e3:d2:64:f1:e4:e2",
		"048c3f6388afb7695dd4d6bbe3d264f1e5e5!",
	}
	mra := mockRARecordingRevocations{}
	log := blog.NewMock()
	a := admin{rac: &mra, log: log}

	assertRequestsContain := func(reqs []*rapb.AdministrativelyRevokeCertificateRequest, code revocation.Reason, skipBlockKey bool, malformed bool) {
		for _, req := range reqs {
			test.AssertEquals(t, len(req.Cert), 0)
			test.AssertEquals(t, req.Code, int64(code))
			test.AssertEquals(t, req.SkipBlockKey, skipBlockKey)
			test.AssertEquals(t, req.Malformed, malformed)
		}
	}

	// Revoking should result in 3 gRPC requests and quiet execution.
	mra.reset()
	log.Clear()
	a.dryRun = false
	err := a.revokeSerials(context.Background(), serials, 0, false, false, 1)
	test.AssertEquals(t, len(log.GetAllMatching("invalid serial format")), 0)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, len(log.GetAll()), 0)
	test.AssertEquals(t, len(mra.revocationRequests), 3)
	assertRequestsContain(mra.revocationRequests, 0, false, false)

	// Revoking an already-revoked serial should result in one log line.
	mra.reset()
	log.Clear()
	mra.alreadyRevoked = []string{"048c3f6388afb7695dd4d6bbe3d264f1e5e5"}
	err = a.revokeSerials(context.Background(), serials, 0, false, false, 1)
	test.AssertError(t, err, "already-revoked should result in error")
	test.AssertEquals(t, len(log.GetAllMatching("not revoking")), 1)
	test.AssertEquals(t, len(mra.revocationRequests), 3)
	assertRequestsContain(mra.revocationRequests, 0, false, false)

	// Revoking a doomed-to-fail serial should also result in one log line.
	mra.reset()
	log.Clear()
	mra.doomedToFail = []string{"048c3f6388afb7695dd4d6bbe3d264f1e5e5"}
	err = a.revokeSerials(context.Background(), serials, 0, false, false, 1)
	test.AssertError(t, err, "gRPC error should result in error")
	test.AssertEquals(t, len(log.GetAllMatching("failed to revoke")), 1)
	test.AssertEquals(t, len(mra.revocationRequests), 3)
	assertRequestsContain(mra.revocationRequests, 0, false, false)

	// Revoking with other parameters should get carried through.
	mra.reset()
	log.Clear()
	err = a.revokeSerials(context.Background(), serials, 1, true, true, 3)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, len(mra.revocationRequests), 3)
	assertRequestsContain(mra.revocationRequests, 1, true, true)

	// Revoking in dry-run mode should result in no gRPC requests and three logs.
	mra.reset()
	log.Clear()
	a.dryRun = true
	a.rac = dryRunRAC{log: log}
	err = a.revokeSerials(context.Background(), serials, 0, false, false, 1)
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, len(log.GetAllMatching("dry-run:")), 3)
	test.AssertEquals(t, len(mra.revocationRequests), 0)
	assertRequestsContain(mra.revocationRequests, 0, false, false)
}
