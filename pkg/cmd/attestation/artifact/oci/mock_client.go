package oci

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/test/data"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func makeTestAttestation() api.Attestation {
	return api.Attestation{Bundle: data.SigstoreBundle(nil)}
}

type MockClient struct{}

func (c MockClient) GetImageDigest(imgName string) (*v1.Hash, name.Reference, error) {
	return &v1.Hash{
		Hex:       "1234567890abcdef",
		Algorithm: "sha256",
	}, nil, nil
}

func (c MockClient) GetAttestations(name name.Reference, digest string) ([]*api.Attestation, error) {
	att1 := makeTestAttestation()
	att2 := makeTestAttestation()
	return []*api.Attestation{&att1, &att2}, nil
}

type ReferenceFailClient struct{}

func (c ReferenceFailClient) GetImageDigest(imgName string) (*v1.Hash, name.Reference, error) {
	return nil, nil, fmt.Errorf("failed to parse reference")
}

func (c ReferenceFailClient) GetAttestations(name name.Reference, digest string) ([]*api.Attestation, error) {
	return nil, nil
}

type AuthFailClient struct{}

func (c AuthFailClient) GetImageDigest(imgName string) (*v1.Hash, name.Reference, error) {
	return nil, nil, ErrRegistryAuthz
}

func (c AuthFailClient) GetAttestations(name name.Reference, digest string) ([]*api.Attestation, error) {
	return nil, nil
}

type DeniedClient struct{}

func (c DeniedClient) GetImageDigest(imgName string) (*v1.Hash, name.Reference, error) {
	return nil, nil, ErrDenied
}

func (c DeniedClient) GetAttestations(name name.Reference, digest string) ([]*api.Attestation, error) {
	return nil, nil
}

type NoAttestationsClient struct{}

func (c NoAttestationsClient) GetImageDigest(imgName string) (*v1.Hash, name.Reference, error) {
	return &v1.Hash{
		Hex:       "1234567890abcdef",
		Algorithm: "sha256",
	}, nil, nil
}

func (c NoAttestationsClient) GetAttestations(name name.Reference, digest string) ([]*api.Attestation, error) {
	return nil, nil
}

type FailedToFetchAttestationsClient struct{}

func (c FailedToFetchAttestationsClient) GetImageDigest(imgName string) (*v1.Hash, name.Reference, error) {
	return &v1.Hash{
		Hex:       "1234567890abcdef",
		Algorithm: "sha256",
	}, nil, nil
}

func (c FailedToFetchAttestationsClient) GetAttestations(name name.Reference, digest string) ([]*api.Attestation, error) {
	return nil, fmt.Errorf("failed to fetch attestations")
}
