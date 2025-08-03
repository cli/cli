package mocks

import (
	"context"

	"google.golang.org/grpc"

	pubpb "github.com/letsencrypt/boulder/publisher/proto"
)

// PublisherClient is a mock
type PublisherClient struct {
	// empty
}

// SubmitToSingleCTWithResult is a mock
func (*PublisherClient) SubmitToSingleCTWithResult(_ context.Context, _ *pubpb.Request, _ ...grpc.CallOption) (*pubpb.Result, error) {
	return &pubpb.Result{}, nil
}
