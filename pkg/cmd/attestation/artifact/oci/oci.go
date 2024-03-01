package oci

import (
	"errors"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

var ErrDenied = errors.New("the provided token was denied access to the requested resource, please check the token's expiration and repository access")
var ErrRegistryAuthz = errors.New("remote registry authorization failed, please authenticate with the registry and try again")

type Client struct {
	ParseReference func(string, ...name.Option) (name.Reference, error)
	Get            func(name.Reference, ...remote.Option) (*remote.Descriptor, error)
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

// where name is formed like ghcr.io/github/my-image-repo
func (c Client) GetImageDigest(imgName string) (*v1.Hash, error) {
	name, err := c.ParseReference(imgName)
	if err != nil {
		return nil, fmt.Errorf("failed to create image tag: %w", err)
	}

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

func NewLiveClient() Client {
	return Client{
		ParseReference: name.ParseReference,
		Get:            remote.Get,
	}
}

func NewMockClient() Client {
	return Client{
		ParseReference: func(string, ...name.Option) (name.Reference, error) {
			return name.Tag{}, nil
		},
		Get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			d := remote.Descriptor{}
			d.Digest = v1.Hash{
				Hex:       "1234567890abcdef",
				Algorithm: "sha256",
			}

			return &d, nil
		},
	}
}

func NewReferenceFailClient() Client {
	return Client{
		ParseReference: func(string, ...name.Option) (name.Reference, error) {
			return nil, fmt.Errorf("failed to parse reference")
		},
		Get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			return nil, nil
		},
	}
}

func NewAuthFailClient() Client {
	return Client{
		ParseReference: func(string, ...name.Option) (name.Reference, error) {
			return name.Tag{}, nil
		},
		Get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			return nil, &transport.Error{Errors: []transport.Diagnostic{{Code: transport.UnauthorizedErrorCode}}}
		},
	}
}

func NewDeniedClient() Client {
	return Client{
		ParseReference: func(string, ...name.Option) (name.Reference, error) {
			return name.Tag{}, nil
		},
		Get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			return nil, &transport.Error{Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode}}}
		},
	}
}
