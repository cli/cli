package live

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/ocsp/responder"
	ocsp_test "github.com/letsencrypt/boulder/ocsp/test"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/test"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/grpc"
)

// mockOCSPGenerator is an ocspGenerator that always emits the provided bytes
// when serial number 1 is requested, but otherwise returns an error.
type mockOCSPGenerator struct {
	resp []byte
}

func (m mockOCSPGenerator) GenerateOCSP(ctx context.Context, in *rapb.GenerateOCSPRequest, opts ...grpc.CallOption) (*capb.OCSPResponse, error) {
	expectedSerial := core.SerialToString(big.NewInt(1))
	if in.Serial != expectedSerial {
		return nil, fmt.Errorf("expected serial %s, got %s", expectedSerial, in.Serial)
	}

	return &capb.OCSPResponse{Response: m.resp}, nil
}

// notFoundOCSPGenerator always returns berrors.NotFound
type notFoundOCSPGenerator struct{}

func (n notFoundOCSPGenerator) GenerateOCSP(ctx context.Context, in *rapb.GenerateOCSPRequest, opts ...grpc.CallOption) (*capb.OCSPResponse, error) {
	return nil, berrors.NotFoundError("not found")
}

func TestLiveResponse(t *testing.T) {
	eeSerial := big.NewInt(1)
	fakeResp, _, _ := ocsp_test.FakeResponse(ocsp.Response{
		SerialNumber: eeSerial,
	})
	source := New(mockOCSPGenerator{fakeResp.Raw}, 1, 0)
	resp, err := source.Response(context.Background(), &ocsp.Request{
		SerialNumber: eeSerial,
	})
	test.AssertNotError(t, err, "getting response")
	test.AssertByteEquals(t, resp.Raw, fakeResp.Raw)
	expectedSerial := "000000000000000000000000000000000001"
	if core.SerialToString(resp.SerialNumber) != expectedSerial {
		t.Errorf("expected serial %s, got %s", expectedSerial, resp.SerialNumber)
	}
}

func TestNotFound(t *testing.T) {
	eeSerial := big.NewInt(1)
	source := New(notFoundOCSPGenerator{}, 1, 0)
	_, err := source.Response(context.Background(), &ocsp.Request{
		SerialNumber: eeSerial,
	})
	if !errors.Is(err, responder.ErrNotFound) {
		t.Errorf("expected responder.ErrNotFound, got %#v", err)
	}
}
