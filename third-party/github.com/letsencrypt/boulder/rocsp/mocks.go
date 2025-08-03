package rocsp

import (
	"context"
	"fmt"

	"golang.org/x/crypto/ocsp"
)

// MockWriteClient is a mock
type MockWriteClient struct {
	StoreResponseReturnError error
}

// StoreResponse mocks a rocsp.StoreResponse method and returns nil or an
// error depending on the desired state.
func (r MockWriteClient) StoreResponse(ctx context.Context, resp *ocsp.Response) error {
	return r.StoreResponseReturnError
}

// NewMockWriteSucceedClient returns a mock MockWriteClient with a
// StoreResponse method that will always succeed.
func NewMockWriteSucceedClient() MockWriteClient {
	return MockWriteClient{nil}
}

// NewMockWriteFailClient returns a mock MockWriteClient with a
// StoreResponse method that will always fail.
func NewMockWriteFailClient() MockWriteClient {
	return MockWriteClient{StoreResponseReturnError: fmt.Errorf("could not store response")}
}
