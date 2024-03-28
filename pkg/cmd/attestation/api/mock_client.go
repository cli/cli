package api

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sigstore/sigstore-go/pkg/bundle"
)

type MockClient struct {
	OnGetByRepoAndDigest  func(repo, digest string, limit int) ([]*Attestation, error)
	OnGetByOwnerAndDigest func(owner, digest string, limit int) ([]*Attestation, error)
}

func (m MockClient) GetByRepoAndDigest(repo, digest string, limit int) ([]*Attestation, error) {
	return m.OnGetByRepoAndDigest(repo, digest, limit)
}

func (m MockClient) GetByOwnerAndDigest(owner, digest string, limit int) ([]*Attestation, error) {
	return m.OnGetByOwnerAndDigest(owner, digest, limit)
}

func makeTestAttestation() Attestation {
	bundleBytes, err := os.ReadFile("../test/data/sigstore-js-2.1.0-bundle.json")
	if err != nil {
		panic(err)
	}

	var b *bundle.ProtobufBundle
	err = json.Unmarshal(bundleBytes, &b)
	if err != nil {
		panic(err)
	}

	return Attestation{Bundle: b}
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
