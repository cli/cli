package verify

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

const hostRegex = `^[a-zA-Z0-9-]+\.[a-zA-Z0-9-]+.*$`

func expandToGitHubURL(tenant, ownerOrRepo string) string {
	if tenant == "" {
		return fmt.Sprintf("https://github.com/%s", ownerOrRepo)
	}
	return fmt.Sprintf("https://%s.ghe.com/%s", tenant, ownerOrRepo)
}

func expandToGitHubURLRegex(tenant, ownerOrRepo string) string {
	url := expandToGitHubURL(tenant, ownerOrRepo)
	return fmt.Sprintf("(?i)^%s/", url)
}

func newEnforcementCriteria(opts *Options) (verification.EnforcementCriteria, error) {
	// initialize the enforcement criteria with the provided PredicateType
	c := verification.EnforcementCriteria{
		PredicateType: opts.PredicateType,
	}

	// set the owner value by checking the repo and owner options
	var owner string
	if opts.Repo != "" {
		// we expect the repo argument to be in the format <OWNER>/<REPO>
		splitRepo := strings.Split(opts.Repo, "/")
		// if Repo is provided but owner is not, set the OWNER portion of the Repo value
		// to Owner
		owner = splitRepo[0]
	} else {
		// otherwise use the user provided owner value
		owner = opts.Owner
	}

	// Set the SANRegex and SAN values using the provided options
	// First check if the opts.SANRegex or opts.SAN values are provided
	if opts.SANRegex != "" || opts.SAN != "" {
		c.SANRegex = opts.SANRegex
		c.SAN = opts.SAN
	} else if opts.SignerRepo != "" {
		// next check if opts.SignerRepo was provided
		signedRepoRegex := expandToGitHubURLRegex(opts.Tenant, opts.SignerRepo)
		c.SANRegex = signedRepoRegex
	} else if opts.SignerWorkflow != "" {
		validatedWorkflowRegex, err := validateSignerWorkflow(opts)
		if err != nil {
			return verification.EnforcementCriteria{}, err
		}
		c.SANRegex = validatedWorkflowRegex
	} else if opts.Repo != "" {
		// if the user has not provided the SAN, SANRegex, SignerRepo, or SignerWorkflow options
		// then we default to the repo option
		c.SANRegex = expandToGitHubURLRegex(opts.Tenant, opts.Repo)
	} else {
		// if opts.Repo was not provided, we fallback to the opts.Owner value
		c.SANRegex = expandToGitHubURLRegex(opts.Tenant, owner)
	}

	// if the DenySelfHostedRunner option is set to true, set the
	// RunnerEnvironment extension to the GitHub hosted runner value
	if opts.DenySelfHostedRunner {
		c.Certificate.RunnerEnvironment = verification.GitHubRunner
	} else {
		// if Certificate.RunnerEnvironment value is set to the empty string
		// through the second function argument,
		// no certificate matching will happen on the RunnerEnvironment field
		c.Certificate.RunnerEnvironment = ""
	}

	// If the Repo option is provided, set the SourceRepositoryURI extension
	if opts.Repo != "" {
		c.Certificate.SourceRepositoryURI = expandToGitHubURL(opts.Tenant, opts.Repo)
	}

	// Set the SourceRepositoryOwnerURI extension using owner and tenant if provided
	c.Certificate.SourceRepositoryOwnerURI = expandToGitHubURL(opts.Tenant, owner)

	// if the tenant is provided and OIDC issuer provided matches the default
	// use the tenant-specific issuer
	if opts.Tenant != "" && opts.OIDCIssuer == verification.GitHubOIDCIssuer {
		c.Certificate.Issuer = fmt.Sprintf(verification.GitHubTenantOIDCIssuer, opts.Tenant)
	} else {
		// otherwise use the custom OIDC issuer provided as an option
		c.Certificate.Issuer = opts.OIDCIssuer
	}

	return c, nil
}

func buildCertificateIdentityOption(c verification.EnforcementCriteria) (verify.PolicyOption, error) {
	sanMatcher, err := verify.NewSANMatcher(c.SAN, c.SANRegex)
	if err != nil {
		return nil, err
	}

	// Accept any issuer, we will verify the issuer as part of the extension verification
	issuerMatcher, err := verify.NewIssuerMatcher("", ".*")
	if err != nil {
		return nil, err
	}

	extensions := certificate.Extensions{
		RunnerEnvironment: c.Certificate.RunnerEnvironment,
	}

	certId, err := verify.NewCertificateIdentity(sanMatcher, issuerMatcher, extensions)
	if err != nil {
		return nil, err
	}

	return verify.WithCertificateIdentity(certId), nil
}

func buildSigstoreVerifyPolicy(c verification.EnforcementCriteria, a artifact.DigestedArtifact) (verify.PolicyBuilder, error) {
	artifactDigestPolicyOption, err := verification.BuildDigestPolicyOption(a)
	if err != nil {
		return verify.PolicyBuilder{}, err
	}

	certIdOption, err := buildCertificateIdentityOption(c)
	if err != nil {
		return verify.PolicyBuilder{}, err
	}

	policy := verify.NewPolicy(artifactDigestPolicyOption, certIdOption)
	return policy, nil
}

func validateSignerWorkflow(opts *Options) (string, error) {
	// we expect a provided workflow argument be in the format [HOST/]/<OWNER>/<REPO>/path/to/workflow.yml
	// if the provided workflow does not contain a host, set the host
	match, err := regexp.MatchString(hostRegex, opts.SignerWorkflow)
	if err != nil {
		return "", err
	}

	if match {
		return fmt.Sprintf("^https://%s", opts.SignerWorkflow), nil
	}

	// if the provided workflow did not match the expect format
	// we move onto creating a signer workflow using the provided host name
	if opts.Hostname == "" {
		return "", errors.New("unknown host")
	}

	return fmt.Sprintf("^https://%s/%s", opts.Hostname, opts.SignerWorkflow), nil
}
