package verify

import (
	"fmt"
	"os"
	"regexp"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

const (
	GitHubOIDCIssuer = "https://token.actions.githubusercontent.com"
	// represents the GitHub hosted runner in the certificate RunnerEnvironment extension
	GitHubRunner = "github-hosted"
	githubHost   = "github.com"
	hostRegex    = `^[a-zA-Z0-9-]+\.[a-zA-Z0-9-]+.*$`
)

func expandToGitHubURL(ownerOrRepo string) string {
	// TODO: handle proxima prefix
	return fmt.Sprintf("(?i)^https://github.com/%s/", ownerOrRepo)
}

func buildSANMatcher(opts *Options) (verify.SubjectAlternativeNameMatcher, error) {
	if opts.SignerRepo != "" {
		signedRepoRegex := expandToGitHubURL(opts.SignerRepo)
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

	issuerMatcher, err := verify.NewIssuerMatcher(opts.OIDCIssuer, "")
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

	// if the provided workflow does not contain a host, check for a host
	// and prepend it to the workflow
	host, err := chooseHost(opts)
	if err != nil {
		return "", err
	}

	return addSchemeToRegex(fmt.Sprintf("%s/%s", host, opts.SignerWorkflow)), nil
}

// if a host was not provided as part of a flag argument choose a host based
// on gh cli configuration
func chooseHost(opts *Options) (string, error) {
	// check if GH_HOST is set and use that host if it is
	host := os.Getenv("GH_HOST")
	if host != "" {
		return host, nil
	}

	// check if the CLI is authenticated with any hosts
	cfg, err := opts.Config()
	if err != nil {
		return "", err
	}

	// if authenticated, return the authenticated host
	authCfg := cfg.Authentication()
	if host, _ := authCfg.DefaultHost(); host != "" {
		return host, nil
	}

	// if not authenticated, return the default host github.com
	return githubHost, nil
}
