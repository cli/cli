package verify

import 	(
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

func buildVerifyPolicy(opts *Options, artifactDigest string) (verify.PolicyBuilder, error) {
	artifactDigestPolicyOption, err := verification.BuildDigestPolicyOption(artifactDigest, opts.DigestAlgorithm)
	if err != nil {
		return verify.PolicyBuilder{}, err
	}

	certIdOption, err := buildVerifyCertIdOption(opts)
	if err != nil {
		return verify.PolicyBuilder{}, err
	}

	policy := verify.NewPolicy(artifactDigestPolicyOption, certIdOption)
	return policy, nil
}
