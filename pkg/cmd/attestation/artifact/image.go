package artifact

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/distribution/reference"
)

func digestContainerImageArtifact(url string, client oci.Client, useBundleFromRegistry bool) (*DigestedArtifact, error) {
	// try to parse the url as a valid registry reference
	named, err := reference.Parse(url)
	if err != nil {
		// cannot be parsed as a registry reference
		return nil, fmt.Errorf("artifact %s is not a valid registry reference: %v", url, err)
	}

	name, err := client.ParseReference(named.String())
	if err != nil {
		return nil, err
	}

	digest, err := client.GetImageDigest(name)

	if err != nil {
		return nil, err
	}
	if useBundleFromRegistry {
		attestations, err := client.GetAttestations(name, digest)

		if err != nil {
			return nil, err
		}

		return &DigestedArtifact{
			URL:          fmt.Sprintf("oci://%s", named.String()),
			digest:       digest.Hex,
			digestAlg:    digest.Algorithm,
			attestations: attestations,
		}, nil
	}

	if err != nil {
		return nil, err
	}

	return &DigestedArtifact{
		URL:       fmt.Sprintf("oci://%s", named.String()),
		digest:    digest.Hex,
		digestAlg: digest.Algorithm,
	}, nil
}
