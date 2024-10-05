package oci

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sigstore/sigstore-go/pkg/bundle"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

var ErrDenied = errors.New("the provided token was denied access to the requested resource, please check the token's expiration and repository access")
var ErrRegistryAuthz = errors.New("remote registry authorization failed, please authenticate with the registry and try again")

type Client interface {
	GetImageDigest(imgName string) (*v1.Hash, name.Reference, error)
	GetAttestations(name name.Reference, digest string) ([]*api.Attestation, error)
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
func (c LiveClient) GetImageDigest(imgName string) (*v1.Hash, name.Reference, error) {
	name, err := c.parseReference(imgName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create image tag: %v", err)
	}
	// The user must already be authenticated with the container registry
	// The authn.DefaultKeychain argument indicates that Get should checks the
	// user's configuration for the registry credentials
	desc, err := c.get(name, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		var transportErr *transport.Error
		if errors.As(err, &transportErr) {
			if accessErr := checkForUnauthorizedOrDeniedErr(*transportErr); accessErr != nil {
				return nil, nil, accessErr
			}
		}
		return nil, nil, fmt.Errorf("failed to fetch remote image: %v", err)
	}

	return &desc.Digest, name, nil
}

type noncompliantRegistryTransport struct{}

// RoundTrip will check if a request and associated response fulfill the following:
// 1. The response returns a 406 status code
// 2. The request path contains /referrers/
// If both conditions are met, the response's status code will be overwritten to 404
// This is a temporary solution to handle non compliant registries that return
// an unexpected status code 406 when the go-containerregistry library used
// by this code attempts to make a request to the referrers API.
// The go-containerregistry library can handle 404 response but not a 406 response.
// See the related go-containerregistry issue: https://github.com/google/go-containerregistry/issues/1962
func (a *noncompliantRegistryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp.StatusCode == http.StatusNotAcceptable && strings.Contains(req.URL.Path, "/referrers/") {
		resp.StatusCode = http.StatusNotFound
	}

	return resp, err
}

func (c LiveClient) GetAttestations(ref name.Reference, digest string) ([]*api.Attestation, error) {
	attestations := make([]*api.Attestation, 0)

	transportOpts := []remote.Option{remote.WithTransport(&noncompliantRegistryTransport{}), remote.WithAuthFromKeychain(authn.DefaultKeychain)}
	referrers, err := remote.Referrers(ref.Context().Digest(digest), transportOpts...)
	if err != nil {
		return attestations, fmt.Errorf("error getting referrers: %w", err)
	}
	refManifest, err := referrers.IndexManifest()
	if err != nil {
		return attestations, fmt.Errorf("error getting referrers manifest: %w", err)
	}

	for _, refDesc := range refManifest.Manifests {
		if !strings.HasPrefix(refDesc.ArtifactType, "application/vnd.dev.sigstore.bundle") {
			continue
		}

		refImg, err := remote.Image(ref.Context().Digest(refDesc.Digest.String()), remote.WithAuthFromKeychain(authn.DefaultKeychain))
		if err != nil {
			return attestations, fmt.Errorf("error getting referrer image: %w", err)
		}
		layers, err := refImg.Layers()
		if err != nil {
			return attestations, fmt.Errorf("error getting referrer image: %w", err)
		}

		if len(layers) > 0 {
			layer0, err := layers[0].Uncompressed()
			if err != nil {
				return attestations, fmt.Errorf("error getting referrer image: %w", err)
			}
			defer layer0.Close()

			bundleBytes, err := io.ReadAll(layer0)

			if err != nil {
				return attestations, fmt.Errorf("error getting referrer image: %w", err)
			}

			b := &bundle.Bundle{}
			err = b.UnmarshalJSON(bundleBytes)

			if err != nil {
				return attestations, fmt.Errorf("error unmarshalling bundle: %w", err)
			}

			a := api.Attestation{Bundle: b}
			attestations = append(attestations, &a)
		} else {
			return attestations, fmt.Errorf("error getting referrer image: no layers found")
		}
	}
	return attestations, nil
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
