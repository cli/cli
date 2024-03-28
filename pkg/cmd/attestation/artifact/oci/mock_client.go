package oci

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/v1"
)

type MockClient struct{}

func (c MockClient) GetImageDigest(imgName string) (*v1.Hash, error) {
	return &v1.Hash{
		Hex:       "1234567890abcdef",
		Algorithm: "sha256",
	}, nil
}

type ReferenceFailClient struct{}

func (c ReferenceFailClient) GetImageDigest(imgName string) (*v1.Hash, error) {
	return nil, fmt.Errorf("failed to parse reference")
}

type AuthFailClient struct{}

func (c AuthFailClient) GetImageDigest(imgName string) (*v1.Hash, error) {
	return nil, ErrRegistryAuthz
}

type DeniedClient struct{}

func (c DeniedClient) GetImageDigest(imgName string) (*v1.Hash, error) {
	return nil, ErrDenied
}
