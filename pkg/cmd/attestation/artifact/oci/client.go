package oci

import (
	"errors"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

var ErrDenied = errors.New("the provided token was denied access to the requested resource, please check the token's expiration and repository access")
var ErrRegistryAuthz = errors.New("remote registry authorization failed, please authenticate with the registry and try again")

type Client interface {
	GetImageDigest(imgName string) (*v1.Hash, error)
}

func checkForUnauthorizedOrDeniedErr(err transport.Error) error {
	for _, diagnostic := range err.Errors {
		switch diagnostic.Code {
		case transport.UnauthorizedErrorCode:
			return ErrRegistryAuthz
		case transport.DeniedErrorCode:
			return ErrDenied
		}
	}
	return nil
}

type LiveClient struct {
	ParseReference func(string, ...name.Option) (name.Reference, error)
	Get            func(name.Reference, ...remote.Option) (*remote.Descriptor, error)
}

// where name is formed like ghcr.io/github/my-image-repo
func (c LiveClient) GetImageDigest(imgName string) (*v1.Hash, error) {
	name, err := c.ParseReference(imgName)
	if err != nil {
		return nil, fmt.Errorf("failed to create image tag: %w", err)
	}

	// The user must already be authenticated with the container registry
	// The authn.DefaultKeychain argument indicates that Get should checks the
	// user's configuration for the registry credentials
	desc, err := c.Get(name, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		var transportErr *transport.Error
		if errors.As(err, &transportErr) {
			if accessErr := checkForUnauthorizedOrDeniedErr(*transportErr); accessErr != nil {
				return nil, accessErr
			}
		}
		return nil, fmt.Errorf("failed to fetch remote image: %w", err)
	}

	return &desc.Digest, nil
}

func NewLiveClient() *LiveClient {
	return &LiveClient{
		ParseReference: name.ParseReference,
		Get:            remote.Get,
	}
}
