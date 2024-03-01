package artifact

import (
	"fmt"

	"github.com/distribution/reference"
	"github.com/github/gh-attestation/pkg/artifact/oci"
)

func digestContainerImageArtifact(url string, client oci.Client) (*DigestedArtifact, error) {
	// try to parse the url as a valid registry reference
	named, err := reference.Parse(url)
	if err != nil {
		// cannot be parsed as a registry reference
		return nil, fmt.Errorf("artifact %s is not a valid registry reference", url)
	}

	digest, err := client.GetImageDigest(named.String())
	if err != nil {
		return nil, err
	}

	return &DigestedArtifact{
		URL:       fmt.Sprintf("oci://%s", named.String()),
		digest:    digest.Hex,
		digestAlg: digest.Algorithm,
	}, nil
}
