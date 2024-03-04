package verification

import (
	"encoding/hex"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"

	"github.com/sigstore/sigstore-go/pkg/verify"
)

// BuildDigestPolicyOption builds a verify.ArtifactPolicyOption
// from the given artifact digest and digest algorithm
func BuildDigestPolicyOption(a artifact.DigestedArtifact) (verify.ArtifactPolicyOption, error) {
	// sigstore-go expects the artifact digest to be decoded from hex
	decoded, err := hex.DecodeString(a.Digest())
	if err != nil {
		return nil, err
	}
	return verify.WithArtifactDigest(a.Algorithm(), decoded), nil
}
