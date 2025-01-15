package api

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
)

type MockClient struct {
	OnGetByRepoAndDigest  func(repo, digest string, limit int) ([]*Attestation, error)
	OnGetByOwnerAndDigest func(owner, digest string, limit int) ([]*Attestation, error)
	OnGetTrustDomain      func() (string, error)
}

func (m MockClient) GetByRepoAndDigest(repo, digest string, limit int) ([]*Attestation, error) {
	return m.OnGetByRepoAndDigest(repo, digest, limit)
}

func (m MockClient) GetByOwnerAndDigest(owner, digest string, limit int) ([]*Attestation, error) {
	return m.OnGetByOwnerAndDigest(owner, digest, limit)
}

func (m MockClient) GetTrustDomain() (string, error) {
	return m.OnGetTrustDomain()
}

func makeTestAttestation() Attestation {
	return Attestation{Bundle: data.SigstoreBundle(nil), BundleURL: "https://example.com"}
}

func OnGetByRepoAndDigestSuccess(repo, digest string, limit int) ([]*Attestation, error) {
	att1 := makeTestAttestation()
	att2 := makeTestAttestation()
	return []*Attestation{&att1, &att2}, nil
}

func OnGetByRepoAndDigestFailure(repo, digest string, limit int) ([]*Attestation, error) {
	return nil, fmt.Errorf("failed to fetch by repo and digest")
}

func OnGetByOwnerAndDigestSuccess(owner, digest string, limit int) ([]*Attestation, error) {
	att1 := makeTestAttestation()
	att2 := makeTestAttestation()
	return []*Attestation{&att1, &att2}, nil
}

func OnGetByOwnerAndDigestFailure(owner, digest string, limit int) ([]*Attestation, error) {
	return nil, fmt.Errorf("failed to fetch by owner and digest")
}

func NewTestClient() *MockClient {
	return &MockClient{
		OnGetByRepoAndDigest:  OnGetByRepoAndDigestSuccess,
		OnGetByOwnerAndDigest: OnGetByOwnerAndDigestSuccess,
	}
}

func NewFailTestClient() *MockClient {
	return &MockClient{
		OnGetByRepoAndDigest:  OnGetByRepoAndDigestFailure,
		OnGetByOwnerAndDigest: OnGetByOwnerAndDigestFailure,
	}
}
