package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/mocks"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
)

func TestSPKIHashFromPrivateKey(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating test private key")
	keyHash, err := core.KeyDigest(privKey.Public())
	test.AssertNotError(t, err, "computing test SPKI hash")

	keyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	test.AssertNotError(t, err, "marshalling test private key bytes")
	keyFile := path.Join(t.TempDir(), "key.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})
	err = os.WriteFile(keyFile, keyPEM, os.ModeAppend)
	test.AssertNotError(t, err, "writing test private key file")

	a := admin{}

	res, err := a.spkiHashFromPrivateKey(keyFile)
	test.AssertNotError(t, err, "")
	test.AssertByteEquals(t, res, keyHash[:])
}

func TestSPKIHashesFromFile(t *testing.T) {
	var spkiHexes []string
	for i := range 10 {
		h := sha256.Sum256([]byte(strconv.Itoa(i)))
		spkiHexes = append(spkiHexes, hex.EncodeToString(h[:]))
	}

	spkiFile := path.Join(t.TempDir(), "spkis.txt")
	err := os.WriteFile(spkiFile, []byte(strings.Join(spkiHexes, "\n")), os.ModeAppend)
	test.AssertNotError(t, err, "writing test spki file")

	a := admin{}

	res, err := a.spkiHashesFromFile(spkiFile)
	test.AssertNotError(t, err, "")
	for i, spkiHash := range res {
		test.AssertEquals(t, hex.EncodeToString(spkiHash), spkiHexes[i])
	}
}

// mockSARecordingBlocks is a mock which only implements the AddBlockedKey gRPC
// method.
type mockSARecordingBlocks struct {
	sapb.StorageAuthorityClient
	blockRequests []*sapb.AddBlockedKeyRequest
}

// AddBlockedKey is a mock which always succeeds and records the request it
// received.
func (msa *mockSARecordingBlocks) AddBlockedKey(ctx context.Context, req *sapb.AddBlockedKeyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	msa.blockRequests = append(msa.blockRequests, req)
	return &emptypb.Empty{}, nil
}

func (msa *mockSARecordingBlocks) reset() {
	msa.blockRequests = nil
}

type mockSARO struct {
	sapb.StorageAuthorityReadOnlyClient
}

func (sa *mockSARO) GetSerialsByKey(ctx context.Context, _ *sapb.SPKIHash, _ ...grpc.CallOption) (grpc.ServerStreamingClient[sapb.Serial], error) {
	return &mocks.ServerStreamClient[sapb.Serial]{}, nil
}

func (sa *mockSARO) KeyBlocked(ctx context.Context, req *sapb.SPKIHash, _ ...grpc.CallOption) (*sapb.Exists, error) {
	return &sapb.Exists{Exists: false}, nil
}

func TestBlockSPKIHash(t *testing.T) {
	fc := clock.NewFake()
	fc.Set(time.Now())
	log := blog.NewMock()
	msa := mockSARecordingBlocks{}

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating test private key")
	keyHash, err := core.KeyDigest(privKey.Public())
	test.AssertNotError(t, err, "computing test SPKI hash")

	a := admin{saroc: &mockSARO{}, sac: &msa, clk: fc, log: log}
	u := &user.User{}

	// A full run should result in one request with the right fields.
	msa.reset()
	log.Clear()
	a.dryRun = false
	err = a.blockSPKIHash(context.Background(), keyHash[:], u, "hello world")
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, len(log.GetAllMatching("Found 0 unexpired certificates")), 1)
	test.AssertEquals(t, len(msa.blockRequests), 1)
	test.AssertByteEquals(t, msa.blockRequests[0].KeyHash, keyHash[:])
	test.AssertContains(t, msa.blockRequests[0].Comment, "hello world")

	// A dry-run should result in zero requests and two log lines.
	msa.reset()
	log.Clear()
	a.dryRun = true
	a.sac = dryRunSAC{log: log}
	err = a.blockSPKIHash(context.Background(), keyHash[:], u, "")
	test.AssertNotError(t, err, "")
	test.AssertEquals(t, len(log.GetAllMatching("Found 0 unexpired certificates")), 1)
	test.AssertEquals(t, len(log.GetAllMatching("dry-run:")), 1)
	test.AssertEquals(t, len(msa.blockRequests), 0)
}
