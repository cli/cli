package oci

import (
	"fmt"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/stretchr/testify/require"
)

func TestGetImageDigest_Success(t *testing.T) {
	expectedDigest := v1.Hash{
		Hex:       "1234567890abcdef",
		Algorithm: "sha256",
	}

	c := LiveClient{
		parseReference: func(string, ...name.Option) (name.Reference, error) {
			return name.Tag{}, nil
		},
		get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			d := remote.Descriptor{}
			d.Digest = expectedDigest

			return &d, nil
		},
	}

	digest, nameRef, err := c.GetImageDigest("test")
	require.NoError(t, err)
	require.Equal(t, &expectedDigest, digest)
	require.Equal(t, name.Tag{}, nameRef)
}

func TestGetImageDigest_ReferenceFail(t *testing.T) {
	c := LiveClient{
		parseReference: func(string, ...name.Option) (name.Reference, error) {
			return nil, fmt.Errorf("failed to parse reference")
		},
		get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			return nil, nil
		},
	}

	digest, nameRef, err := c.GetImageDigest("test")
	require.Error(t, err)
	require.Nil(t, digest)
	require.Nil(t, nameRef)
}

func TestGetImageDigest_AuthFail(t *testing.T) {
	c := LiveClient{
		parseReference: func(string, ...name.Option) (name.Reference, error) {
			return name.Tag{}, nil
		},
		get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			return nil, &transport.Error{Errors: []transport.Diagnostic{{Code: transport.UnauthorizedErrorCode}}}
		},
	}

	digest, nameRef, err := c.GetImageDigest("test")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRegistryAuthz)
	require.Nil(t, digest)
	require.Nil(t, nameRef)
}

func TestGetImageDigest_Denied(t *testing.T) {
	c := LiveClient{
		parseReference: func(string, ...name.Option) (name.Reference, error) {
			return name.Tag{}, nil
		},
		get: func(name.Reference, ...remote.Option) (*remote.Descriptor, error) {
			return nil, &transport.Error{Errors: []transport.Diagnostic{{Code: transport.DeniedErrorCode}}}
		},
	}

	digest, nameRef, err := c.GetImageDigest("test")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDenied)
	require.Nil(t, digest)
	require.Nil(t, nameRef)
}
