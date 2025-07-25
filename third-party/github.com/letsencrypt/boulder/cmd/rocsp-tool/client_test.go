package notmain

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/grpc"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/cmd"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/db"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/rocsp"
	"github.com/letsencrypt/boulder/sa"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/test/vars"
)

func makeClient() (*rocsp.RWClient, clock.Clock) {
	CACertFile := "../../test/certs/ipki/minica.pem"
	CertFile := "../../test/certs/ipki/localhost/cert.pem"
	KeyFile := "../../test/certs/ipki/localhost/key.pem"
	tlsConfig := cmd.TLSConfig{
		CACertFile: CACertFile,
		CertFile:   CertFile,
		KeyFile:    KeyFile,
	}
	tlsConfig2, err := tlsConfig.Load(metrics.NoopRegisterer)
	if err != nil {
		panic(err)
	}

	rdb := redis.NewRing(&redis.RingOptions{
		Addrs: map[string]string{
			"shard1": "10.77.77.2:4218",
			"shard2": "10.77.77.3:4218",
		},
		Username:  "unittest-rw",
		Password:  "824968fa490f4ecec1e52d5e34916bdb60d45f8d",
		TLSConfig: tlsConfig2,
	})
	clk := clock.NewFake()
	return rocsp.NewWritingClient(rdb, 500*time.Millisecond, clk, metrics.NoopRegisterer), clk
}

func insertCertificateStatus(t *testing.T, dbMap db.Executor, serial string, notAfter, ocspLastUpdated time.Time) int64 {
	result, err := dbMap.ExecContext(context.Background(),
		`INSERT INTO certificateStatus
		(serial, notAfter, status, ocspLastUpdated, revokedDate, revokedReason, lastExpirationNagSent, issuerID)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		serial,
		notAfter,
		core.OCSPStatusGood,
		ocspLastUpdated,
		time.Time{},
		0,
		time.Time{},
		99)
	test.AssertNotError(t, err, "inserting certificate status")
	id, err := result.LastInsertId()
	test.AssertNotError(t, err, "getting last insert ID")
	return id
}

func TestGetStartingID(t *testing.T) {
	clk := clock.NewFake()
	dbMap, err := sa.DBMapForTest(vars.DBConnSAFullPerms)
	test.AssertNotError(t, err, "failed setting up db client")
	defer test.ResetBoulderTestDatabase(t)()

	firstID := insertCertificateStatus(t, dbMap, "1337", clk.Now().Add(12*time.Hour), time.Time{})
	secondID := insertCertificateStatus(t, dbMap, "1338", clk.Now().Add(36*time.Hour), time.Time{})

	t.Logf("first ID %d, second ID %d", firstID, secondID)

	clk.Sleep(48 * time.Hour)

	startingID, err := getStartingID(context.Background(), clk, dbMap)
	test.AssertNotError(t, err, "getting starting ID")

	test.AssertEquals(t, startingID, secondID)
}

func TestStoreResponse(t *testing.T) {
	redisClient, clk := makeClient()

	issuer, err := core.LoadCert("../../test/hierarchy/int-e1.cert.pem")
	test.AssertNotError(t, err, "loading int-e1")

	issuerKey, err := test.LoadSigner("../../test/hierarchy/int-e1.key.pem")
	test.AssertNotError(t, err, "loading int-e1 key ")
	response, err := ocsp.CreateResponse(issuer, issuer, ocsp.Response{
		SerialNumber: big.NewInt(1337),
		Status:       0,
		ThisUpdate:   clk.Now(),
		NextUpdate:   clk.Now().Add(time.Hour),
	}, issuerKey)
	test.AssertNotError(t, err, "creating OCSP response")

	cl := client{
		redis:         redisClient,
		db:            nil,
		ocspGenerator: nil,
		clk:           clk,
		logger:        blog.NewMock(),
	}

	err = cl.storeResponse(context.Background(), response)
	test.AssertNotError(t, err, "storing response")
}

type mockOCSPGenerator struct{}

func (mog mockOCSPGenerator) GenerateOCSP(ctx context.Context, in *capb.GenerateOCSPRequest, opts ...grpc.CallOption) (*capb.OCSPResponse, error) {
	return &capb.OCSPResponse{
		Response: []byte("phthpbt"),
	}, nil

}

func TestLoadFromDB(t *testing.T) {
	redisClient, clk := makeClient()

	dbMap, err := sa.DBMapForTest(vars.DBConnSA)
	if err != nil {
		t.Fatalf("Failed to create dbMap: %s", err)
	}

	defer test.ResetBoulderTestDatabase(t)

	for i := range 100 {
		insertCertificateStatus(t, dbMap, fmt.Sprintf("%036x", i), clk.Now().Add(200*time.Hour), clk.Now())
		if err != nil {
			t.Fatalf("Failed to insert certificateStatus: %s", err)
		}
	}

	rocspToolClient := client{
		redis:         redisClient,
		db:            dbMap,
		ocspGenerator: mockOCSPGenerator{},
		clk:           clk,
		scanBatchSize: 10,
		logger:        blog.NewMock(),
	}

	speed := ProcessingSpeed{
		RowsPerSecond: 10000,
		ParallelSigns: 100,
	}

	err = rocspToolClient.loadFromDB(context.Background(), speed, 0)
	if err != nil {
		t.Fatalf("loading from DB: %s", err)
	}
}
