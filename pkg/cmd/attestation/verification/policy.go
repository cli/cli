package verify

import (
	"encoding/hex"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const (
	GitHubOIDCIssuer  = "https://token.actions.githubusercontent.com"
	SLSAPredicateType = "https://slsa.dev/provenance/v1"
	// represents the GitHub hosted runner in the certificate RunnerEnvironment extension
	GitHubRunner = "github-hosted"
)

func buildSANMatcher(opts *VerifyOpts) (verify.SubjectAlternativeNameMatcher, error) {
	if opts.SAN != "" || opts.SANRegex != "" {
		sanMatcher, err := verify.NewSANMatcher(opts.SAN, "", opts.SANRegex)
		if err != nil {
			return verify.SubjectAlternativeNameMatcher{}, err
		}
		return sanMatcher, nil
	}
	return verify.SubjectAlternativeNameMatcher{}, nil
}

func buildCertificateIdentityOption(opts *VerifyOpts, runnerEnv string) (verify.PolicyOption, error) {
	sanMatcher, err := buildSANMatcher(opts)
	if err != nil {
		return nil, err
	}

	extensions := certificate.Extensions{
		Issuer:                   opts.OIDCIssuer,
		SourceRepositoryOwnerURI: fmt.Sprintf("https://github.com/%s", opts.Owner),
		RunnerEnvironment:        runnerEnv,
	}
	certId, err := verify.NewCertificateIdentity(sanMatcher, extensions)
	if err != nil {
		return nil, err
	}

	return verify.WithCertificateIdentity(certId), nil
}

func buildVerifyCertIdOption(opts *VerifyOpts) (verify.PolicyOption, error) {
	if opts.DenySelfHostedRunner {
		withGHRunner, err := buildCertificateIdentityOption(opts, GitHubRunner)
		if err != nil {
			return nil, err
		}

		return withGHRunner, nil
	}

	// if Extensions.RunnerEnvironment value is set to the empty string
	// through the second function argument,
	// no certificate matching will happen on the RunnerEnvironment field
	withAnyRunner, err := buildCertificateIdentityOption(opts, "")
	if err != nil {
		return nil, err
	}

	return withAnyRunner, nil
}

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
