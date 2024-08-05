package oci

import (
	"errors"
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/sigstore-go/pkg/bundle"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
)

var ErrDenied = errors.New("the provided token was denied access to the requested resource, please check the token's expiration and repository access")
var ErrRegistryAuthz = errors.New("remote registry authorization failed, please authenticate with the registry and try again")

type Client interface {
	GetImageDigest(name name.Reference) (*v1.Hash, error)
	GetAttestations(name name.Reference, digest *v1.Hash) ([]*api.Attestation, error)
	ParseReference(ref string) (name.Reference, error)
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
	parseReference func(string, ...name.Option) (name.Reference, error)
	get            func(name.Reference, ...remote.Option) (*remote.Descriptor, error)
}

func (c LiveClient) ParseReference(ref string) (name.Reference, error) {
	return c.parseReference(ref)
}

// where name is formed like ghcr.io/github/my-image-repo
func (c LiveClient) GetImageDigest(name name.Reference) (*v1.Hash, error) {
	// The user must already be authenticated with the container registry
	// The authn.DefaultKeychain argument indicates that Get should checks the
	// user's configuration for the registry credentials
	desc, err := c.get(name, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		var transportErr *transport.Error
		if errors.As(err, &transportErr) {
			if accessErr := checkForUnauthorizedOrDeniedErr(*transportErr); accessErr != nil {
				return nil, accessErr
			}
		}
		return nil, fmt.Errorf("failed to fetch remote image: %v", err)
	}

	return &desc.Digest, nil
}

// Ref: https://github.com/github/package-security/blob/main/garden/retrieve-sigstore-bundle-from-oci-registry.md
func (c LiveClient) GetAttestations(name name.Reference, digest *v1.Hash) ([]*api.Attestation, error) {
	attestations := []*api.Attestation{}
	nameDegist := name.Context().Digest(digest.String())

	imageIndex, err := remote.Referrers(nameDegist, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return attestations, fmt.Errorf("failed to fetch remote image: %v", err)
	}
	indexManifest, err := imageIndex.IndexManifest()
	if err != nil {
		return attestations, fmt.Errorf("failed to fetch remote image: %v", err)
	}
	manifests := indexManifest.Manifests

	for _, m := range manifests {

		if containAllowedArtifactTypes(m.ArtifactType) {
			manifestDigest := m.Digest.String()

			digest2 := nameDegist.Context().Digest(manifestDigest)
			// TODO: replace to use GET for more correct type
			// OR IS IT CORRECT TO USE type IMAGE?
			img2, err := remote.Image(digest2, remote.WithAuthFromKeychain(authn.DefaultKeychain))
			if err != nil {
				return attestations, fmt.Errorf("failed to fetch remote image: %v", err)
			}

			// Step 4: Get the layers
			layers, err := img2.Layers()
			if err != nil {
				return attestations, fmt.Errorf("failed to fetch remote image: %v", err)

			}

			// For simplicity, we'll just fetch the first layer
			if len(layers) > 0 {
				layer := layers[0]

				// Step 5: Read the blob (layer) content
				rc, err := layer.Compressed()
				if err != nil {
					return attestations, fmt.Errorf("failed to fetch remote image: %v", err)
				}
				defer rc.Close()

				layerBytes, err := io.ReadAll(rc)

				if err != nil {
					return attestations, fmt.Errorf("failed to fetch remote image: %v", err)
				}

				var bundle bundle.ProtobufBundle
				bundle.Bundle = new(protobundle.Bundle)
				err = bundle.UnmarshalJSON(layerBytes)

				if err != nil {
					return attestations, fmt.Errorf("failed to fetch remote image: %v", err)
				}

				a := api.Attestation{Bundle: &bundle}
				attestations = append(attestations, &a)

			} else {
				return attestations, fmt.Errorf("failed to fetch remote image: %v", err)
			}
		}
	}
	return attestations, nil
}

func containAllowedArtifactTypes(artifactType string) bool {
	allowedArtifactTypes := []string{"application/vnd.dev.sigstore.bundle.v0.3+json"}

	for _, allowedType := range allowedArtifactTypes {
		if allowedType == artifactType {
			return true
		}
	}
	return false
}

// Unlike other parts of this command set, we cannot pass a custom HTTP client
// to the go-containerregistry library. This means we have limited visibility
// into the HTTP requests being made to container registries.
func NewLiveClient() *LiveClient {
	return &LiveClient{
		parseReference: name.ParseReference,
		get:            remote.Get,
	}
}
