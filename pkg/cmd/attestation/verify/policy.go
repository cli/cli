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

// signerWorkflowRefPattern matches the "@<ref>" that terminates the
// SubjectAlternativeName of a certificate issued to a GitHub Actions workflow.
//
// The separator cannot be located by splitting on "@", because both sides may contain
// one. A git ref may contain "@", as in the refs/tags/pkg@1.2.3 convention used by
// JavaScript monorepos, and so may a workflow file name. What separates them is shape:
// a ref is either a "refs/"-prefixed path or a bare object ID, and a workflow file name
// can be neither. It cannot contain "/", since Actions only loads workflows stored
// directly in .github/workflows, and it cannot be a bare object ID, since it must carry
// a .yml or .yaml extension.
const signerWorkflowRefPattern = `@(refs/.*|[0-9a-fA-F]+)`

// signerWorkflowHasRef reports whether a --signer-workflow value already pins a ref.
var signerWorkflowHasRef = regexp.MustCompile(signerWorkflowRefPattern + `$`)

// signerWorkflowSANPattern builds the SAN pattern enforced for --signer-workflow.
//
// sigstore-go matches SAN patterns with regexp.MatchString, which is unanchored, so a
// pattern anchored only at the start is a prefix match: a workflow named
// release.yml.attacker.yml satisfies a pin on release.yml. Anchoring the end makes the
// pin an identity match.
//
// The documented flag format carries no ref, and such a value has to keep matching a
// certificate from any ref, so the ref is optional. Once the value pins a ref of its
// own it must match the end of the SAN exactly. Leaving the ref optional there would
// let a second ref follow the pinned one, and since a branch may be named
// "main@refs/heads/attacker", a pin on refs/heads/main would still admit a certificate
// built from a different branch.
func signerWorkflowSANPattern(workflowURL string) string {
	pattern := "^" + regexp.QuoteMeta(workflowURL)
	if signerWorkflowHasRef.MatchString(workflowURL) {
		return pattern + "$"
	}
	return pattern + "(" + signerWorkflowRefPattern + ")?$"
}

func expandToGitHubURL(tenant, ownerOrRepo string) string {
	if tenant == "" {
		return fmt.Sprintf("https://github.com/%s", ownerOrRepo)
	}
	return fmt.Sprintf("https://%s.ghe.com/%s", tenant, ownerOrRepo)
}

func expandToGitHubURLRegex(tenant, ownerOrRepo string) string {
	url := expandToGitHubURL(tenant, ownerOrRepo)
	return fmt.Sprintf("(?i)^%s", regexp.QuoteMeta(url+"/"))
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
		validatedWorkflowRegex, err := validateSignerWorkflow(opts.Hostname, opts.SignerWorkflow)
		if err != nil {
			return verification.EnforcementCriteria{}, err
		}
		c.SANRegex = validatedWorkflowRegex
	} else if opts.Repo != "" {
		// if the user has not provided the SAN, SANRegex, SignerRepo, or SignerWorkflow options
		// then we default to the repo option
		c.SANRegex = expandToGitHubURLRegex(opts.Tenant, opts.Repo)
	} else {
		// if opts.Repo was not provided, we fall back to the opts.Owner value
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

	// set the SourceRepositoryDigest, SourceRepositoryRef, and BuildSignerDigest
	// extensions if the options are provided
	c.Certificate.BuildSignerDigest = opts.SignerDigest
	c.Certificate.SourceRepositoryDigest = opts.SourceDigest
	c.Certificate.SourceRepositoryRef = opts.SourceRef

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

func validateSignerWorkflow(hostname, signerWorkflow string) (string, error) {
	// we expect a provided workflow argument be in the format [HOST/]/<OWNER>/<REPO>/path/to/workflow.yml
	// if the provided workflow does not contain a host, set the host
	match, err := regexp.MatchString(hostRegex, signerWorkflow)
	if err != nil {
		return "", err
	}

	if match {
		return signerWorkflowSANPattern(fmt.Sprintf("https://%s", signerWorkflow)), nil
	}

	// if the provided workflow did not match the expect format
	// we move onto creating a signer workflow using the provided host name
	if hostname == "" {
		return "", errors.New("unknown signer workflow host")
	}

	return signerWorkflowSANPattern(fmt.Sprintf("https://%s/%s", hostname, signerWorkflow)), nil
}
