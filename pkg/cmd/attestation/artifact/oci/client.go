package oci

import (
	"bytes"
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
	GetImageDigest(imgName string) (*v1.Hash, []*api.Attestation, error)
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

// where name is formed like ghcr.io/github/my-image-repo
func (c LiveClient) GetImageDigest(imgName string) (*v1.Hash, []*api.Attestation, error) {
	nameFirst, err := c.parseReference(imgName)
	var buf bytes.Buffer
	attestations := []*api.Attestation{}

	if err != nil {
		return nil, attestations, fmt.Errorf("failed to create image tag: %v", err)
	}

	// The user must already be authenticated with the container registry
	// The authn.DefaultKeychain argument indicates that Get should checks the
	// user's configuration for the registry credentials
	desc, err := c.get(nameFirst, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		var transportErr *transport.Error
		if errors.As(err, &transportErr) {
			if accessErr := checkForUnauthorizedOrDeniedErr(*transportErr); accessErr != nil {
				return nil, attestations, accessErr
			}
		}
		return nil, attestations, fmt.Errorf("failed to fetch remote image: %v", err)
	}

	dgst := nameFirst.Context().Digest(desc.Digest.String())

	ref, err := remote.Referrers(dgst, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	indexManifest, err := ref.IndexManifest()
	if err != nil {
		return nil, attestations, fmt.Errorf("failed to fetch remote image: %v", err)
	}
	manifests := indexManifest.Manifests

	for _, m := range manifests {
		allowedArtifactTypes := []string{"application/vnd.dev.sigstore.bundle.v0.3+json"}

		for _, allowedType := range allowedArtifactTypes {
			if allowedType == m.ArtifactType {
				manifestDigest := m.Digest.String()

				manifestURL := fmt.Sprintf("%s/manifests/%s", imgName, manifestDigest)
				fmt.Println(manifestURL)

				digest2 := nameFirst.Context().Digest(manifestDigest)
				// replace to use GET for more correc type
				img2, err := remote.Image(digest2, remote.WithAuthFromKeychain(authn.DefaultKeychain))
				if err != nil {
					return nil, attestations, fmt.Errorf("failed to fetch remote image: %v", err)
				}
				// manifest2, err := ref2.Manifest()
				// if err != nil {
				// 	return nil, fmt.Errorf("failed to fetch remote image: %v", err)
				// }

				// fmt.Println(manifest2.MediaType)
				// Step 4: Get the layers
				layers, err := img2.Layers()
				if err != nil {
					fmt.Println("Error getting layers: %v", err)
					return nil, attestations, fmt.Errorf("failed to fetch remote image: %v", err)

				}

				// For simplicity, we'll just fetch the first layer
				if len(layers) > 0 {
					fmt.Println("how many layers?", len(layers))
					layer := layers[0]

					// Step 5: Read the blob (layer) content
					rc, err := layer.Compressed()
					if err != nil {
						fmt.Println("Error getting compressed layer: %v", err)
						return nil, attestations, fmt.Errorf("failed to fetch remote image: %v", err)

					}
					defer rc.Close()

					layerBytes, err := io.ReadAll(rc)

					if err != nil {
						// If creating a gzip reader fails, it might not be compressed
						fmt.Println("Layer is not gzip-compressed. Reading raw content.")
						// fmt.Println("Blob content:", buf.String())

						var bundle bundle.ProtobufBundle
						bundle.Bundle = new(protobundle.Bundle)
						err = bundle.UnmarshalJSON(layerBytes)
						fmt.Println("")
						fmt.Println("JSON Content:", string(layerBytes))
						fmt.Println("")

						if err != nil {
							fmt.Println("failed to unmarshal bundle from JSON: %v", err)
						} else {
							fmt.Println("Bundle content:", bundle.String())
						}

						a := api.Attestation{Bundle: &bundle}
						attestations = append(attestations, &a)

					} else {
						defer gz.Close()

						var decompressed bytes.Buffer
						if _, err := io.Copy(&decompressed, gz); err != nil {
							fmt.Println("Error decompressing layer content: %v", err)
						}

						// Now you have the decompressed blob content in 'decompressed' buffer
						fmt.Println("Decompressed blob content:", decompressed.String())
					}

					// Now you have the decompressed blob content in 'decompressed' buffer
					// fmt.Println("Blob content:", decompressed.String())
				} else {
					fmt.Println("No layers found in the image.")
				}
			}
		}
	}

	// msgName, err := c.parseReference(msg)
	// fmt.Println()
	// if err != nil {
	// 	return nil, err
	// }

	return &desc.Digest, attestations, nil
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
