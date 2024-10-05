package verify

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

const (
	// represents the GitHub hosted runner in the certificate RunnerEnvironment extension
	GitHubRunner = "github-hosted"
	hostRegex    = `^[a-zA-Z0-9-]+\.[a-zA-Z0-9-]+.*$`
)

func expandToGitHubURL(tenant, ownerOrRepo string) string {
	if tenant == "" {
		return fmt.Sprintf("(?i)^https://github.com/%s/", ownerOrRepo)
	}
	return fmt.Sprintf("(?i)^https://%s.ghe.com/%s/", tenant, ownerOrRepo)
}

func buildSANMatcher(opts *Options) (verify.SubjectAlternativeNameMatcher, error) {
	if opts.SignerRepo != "" {
		signedRepoRegex := expandToGitHubURL(opts.Tenant, opts.SignerRepo)
		return verify.NewSANMatcher("", signedRepoRegex)
	} else if opts.SignerWorkflow != "" {
		validatedWorkflowRegex, err := validateSignerWorkflow(opts)
		if err != nil {
			return verify.SubjectAlternativeNameMatcher{}, err
		}

		return verify.NewSANMatcher("", validatedWorkflowRegex)
	} else if opts.SAN != "" || opts.SANRegex != "" {
		return verify.NewSANMatcher(opts.SAN, opts.SANRegex)
	}

	return verify.SubjectAlternativeNameMatcher{}, nil
}

func buildCertificateIdentityOption(opts *Options, runnerEnv string) (verify.PolicyOption, error) {
	sanMatcher, err := buildSANMatcher(opts)
	if err != nil {
		return nil, err
	}

	// Accept any issuer, we will verify the issuer as part of the extension verification
	issuerMatcher, err := verify.NewIssuerMatcher("", ".*")
	if err != nil {
		return nil, err
	}

	extensions := certificate.Extensions{
		RunnerEnvironment: runnerEnv,
	}

	certId, err := verify.NewCertificateIdentity(sanMatcher, issuerMatcher, extensions)
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

func addSchemeToRegex(s string) string {
	return fmt.Sprintf("^https://%s", s)
}

func validateSignerWorkflow(opts *Options) (string, error) {
	// we expect a provided workflow argument be in the format [HOST/]/<OWNER>/<REPO>/path/to/workflow.yml
	// if the provided workflow does not contain a host, set the host
	match, err := regexp.MatchString(hostRegex, opts.SignerWorkflow)
	if err != nil {
		return "", err
	}

	if match {
		return addSchemeToRegex(opts.SignerWorkflow), nil
	}

	if opts.Hostname == "" {
		return "", errors.New("unknown host")
	}

	return addSchemeToRegex(fmt.Sprintf("%s/%s", opts.Hostname, opts.SignerWorkflow)), nil
}
