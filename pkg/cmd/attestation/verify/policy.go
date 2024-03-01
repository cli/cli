package verify

import 	(
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"

	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

const (
	GitHubOIDCIssuer  = "https://token.actions.githubusercontent.com"
	SLSAPredicateType = "https://slsa.dev/provenance/v1"
	// represents the GitHub hosted runner in the certificate RunnerEnvironment extension
	GitHubRunner = "github-hosted"
)

func buildSANMatcher(opts *Options) (verify.SubjectAlternativeNameMatcher, error) {
	if opts.SAN != "" || opts.SANRegex != "" {
		sanMatcher, err := verify.NewSANMatcher(opts.SAN, "", opts.SANRegex)
		if err != nil {
			return verify.SubjectAlternativeNameMatcher{}, err
		}
		return sanMatcher, nil
	}
	return verify.SubjectAlternativeNameMatcher{}, nil
}

func buildCertificateIdentityOption(opts *Options, runnerEnv string) (verify.PolicyOption, error) {
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

func buildVerifyCertIdOption(opts *Options) (verify.PolicyOption, error) {
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
