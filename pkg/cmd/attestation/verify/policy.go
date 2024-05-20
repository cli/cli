package verify

import (
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

const (
	GitHubOIDCIssuer = "https://token.actions.githubusercontent.com"
	// represents the GitHub hosted runner in the certificate RunnerEnvironment extension
	GitHubRunner = "github-hosted"
)

func buildSANMatcher(san, sanRegex string) (verify.SubjectAlternativeNameMatcher, error) {
	if san == "" && sanRegex == "" {
		return verify.SubjectAlternativeNameMatcher{}, nil
	}

	sanMatcher, err := verify.NewSANMatcher(san, "", sanRegex)
	if err != nil {
		return verify.SubjectAlternativeNameMatcher{}, err
	}
	return sanMatcher, nil
}

func buildCertExtensions(opts *Options, runnerEnv string) certificate.Extensions {
	extensions := certificate.Extensions{
		Issuer:                   opts.OIDCIssuer,
		SourceRepositoryOwnerURI: fmt.Sprintf("https://github.com/%s", opts.Owner),
		RunnerEnvironment:        runnerEnv,
	}

	// if opts.Repo is set, set the SourceRepositoryURI field before returning the extensions
	if opts.Repo != "" {
		extensions.SourceRepositoryURI = fmt.Sprintf("https://github.com/%s", opts.Repo)
	}
	return extensions
}

func buildCertificateIdentityOption(opts *Options, runnerEnv string) (verify.PolicyOption, error) {
	sanMatcher, err := buildSANMatcher(opts.SAN, opts.SANRegex)
	if err != nil {
		return nil, err
	}

	extensions := buildCertExtensions(opts, runnerEnv)

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

func buildVerifyPolicy(opts *Options, a artifact.DigestedArtifact) (verify.PolicyBuilder, error) {
	artifactDigestPolicyOption, err := verification.BuildDigestPolicyOption(a)
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
