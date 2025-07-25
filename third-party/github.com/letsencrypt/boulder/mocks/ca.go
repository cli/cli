package mocks

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"google.golang.org/grpc"

	capb "github.com/letsencrypt/boulder/ca/proto"
)

// MockCA is a mock of a CA that always returns the cert from PEM in response to
// IssueCertificate.
type MockCA struct {
	PEM []byte
}

// IssueCertificate is a mock
func (ca *MockCA) IssueCertificate(ctx context.Context, req *capb.IssueCertificateRequest, _ ...grpc.CallOption) (*capb.IssueCertificateResponse, error) {
	if ca.PEM == nil {
		return nil, fmt.Errorf("MockCA's PEM field must be set before calling IssueCertificate")
	}
	block, _ := pem.Decode(ca.PEM)
	sampleDER, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &capb.IssueCertificateResponse{DER: sampleDER.Raw}, nil
}

type MockOCSPGenerator struct{}

// GenerateOCSP is a mock
func (ca *MockOCSPGenerator) GenerateOCSP(ctx context.Context, req *capb.GenerateOCSPRequest, _ ...grpc.CallOption) (*capb.OCSPResponse, error) {
	return nil, nil
}

type MockCRLGenerator struct{}

// GenerateCRL is a mock
func (ca *MockCRLGenerator) GenerateCRL(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[capb.GenerateCRLRequest, capb.GenerateCRLResponse], error) {
	return nil, nil
}
