package inmemnonce

import (
	"context"

	"github.com/go-jose/go-jose/v4"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/letsencrypt/boulder/nonce"
	noncepb "github.com/letsencrypt/boulder/nonce/proto"
)

// Service implements noncepb.NonceServiceClient for tests.
type Service struct {
	*nonce.NonceService
}

var _ noncepb.NonceServiceClient = &Service{}

// Nonce implements proto.NonceServiceClient
func (imns *Service) Nonce(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*noncepb.NonceMessage, error) {
	n, err := imns.NonceService.Nonce()
	if err != nil {
		return nil, err
	}
	return &noncepb.NonceMessage{Nonce: n}, nil
}

// Redeem implements proto.NonceServiceClient
func (imns *Service) Redeem(ctx context.Context, in *noncepb.NonceMessage, opts ...grpc.CallOption) (*noncepb.ValidMessage, error) {
	valid := imns.NonceService.Valid(in.Nonce)
	return &noncepb.ValidMessage{Valid: valid}, nil
}

// AsSource returns a wrapper type that implements jose.NonceSource using this
// inmemory service. This is useful so that tests can get nonces for signing
// their JWS that will be accepted by the test WFE configured using this service.
func (imns *Service) AsSource() jose.NonceSource {
	return nonceServiceAdapter{imns}
}

// nonceServiceAdapter changes the gRPC nonce service interface to the one
// required by jose. Used only for tests.
type nonceServiceAdapter struct {
	noncepb.NonceServiceClient
}

// Nonce returns a nonce, implementing the jose.NonceSource interface
func (nsa nonceServiceAdapter) Nonce() (string, error) {
	resp, err := nsa.NonceServiceClient.Nonce(context.Background(), &emptypb.Empty{})
	if err != nil {
		return "", err
	}
	return resp.Nonce, nil
}

var _ jose.NonceSource = nonceServiceAdapter{}
