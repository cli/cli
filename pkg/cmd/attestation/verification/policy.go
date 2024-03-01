package verify

import (
	"encoding/hex"

	"github.com/sigstore/sigstore-go/pkg/verify"
)

// BuildDigestPolicyOption builds a verify.ArtifactPolicyOption
// from the given artifact digest and digest algorithm
func BuildDigestPolicyOption(artifactDigest, digestAlgorithm string) (verify.ArtifactPolicyOption, error) {
	// sigstore-go expects the artifact digest to be decoded from hex
	decoded, err := hex.DecodeString(artifactDigest)
	if err != nil {
		return nil, err
	}
	return verify.WithArtifactDigest(digestAlgorithm, decoded), nil
}
