package mocks

import (
	"context"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/letsencrypt/boulder/email"
	emailpb "github.com/letsencrypt/boulder/email/proto"
)

// MockPardotClientImpl is a mock implementation of PardotClient.
type MockPardotClientImpl struct {
	sync.Mutex
	CreatedContacts []string
}

// NewMockPardotClientImpl returns a emailPardotClient and a
// MockPardotClientImpl. Both refer to the same instance, with the interface for
// mock interaction and the struct for state inspection and modification.
func NewMockPardotClientImpl() (email.PardotClient, *MockPardotClientImpl) {
	mockImpl := &MockPardotClientImpl{
		CreatedContacts: []string{},
	}
	return mockImpl, mockImpl
}

// SendContact adds an email to CreatedContacts.
func (m *MockPardotClientImpl) SendContact(email string) error {
	m.Lock()
	defer m.Unlock()

	m.CreatedContacts = append(m.CreatedContacts, email)
	return nil
}

// GetCreatedContacts is used for testing to retrieve the list of created
// contacts in a thread-safe manner.
func (m *MockPardotClientImpl) GetCreatedContacts() []string {
	m.Lock()
	defer m.Unlock()
	// Return a copy to avoid race conditions.
	return append([]string{}, m.CreatedContacts...)
}

// MockExporterClientImpl is a mock implementation of ExporterClient.
type MockExporterClientImpl struct {
	PardotClient email.PardotClient
}

// NewMockExporterImpl returns a MockExporterClientImpl as an ExporterClient.
func NewMockExporterImpl(pardotClient email.PardotClient) emailpb.ExporterClient {
	return &MockExporterClientImpl{
		PardotClient: pardotClient,
	}
}

// SendContacts submits emails to the inner PardotClient, returning an error if
// any fail.
func (m *MockExporterClientImpl) SendContacts(ctx context.Context, req *emailpb.SendContactsRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	for _, e := range req.Emails {
		err := m.PardotClient.SendContact(e)
		if err != nil {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}
