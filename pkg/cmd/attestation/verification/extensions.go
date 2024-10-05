package verification

import (
	"errors"
	"fmt"
	"strings"
)

var (
	GitHubOIDCIssuer       = "https://token.actions.githubusercontent.com"
	GitHubTenantOIDCIssuer = "https://token.actions.%s.ghe.com"
)

func VerifyCertExtensions(results []*AttestationProcessingResult, tenant, owner, repo, issuer string) error {
	if len(results) == 0 {
		return errors.New("no attestations proccessing results")
	}

	for _, attestation := range results {
		if err := verifyCertExtensions(attestation, tenant, owner, repo, issuer); err != nil {
			return err
		}
	}
	return nil
}

func verifyCertExtensions(attestation *AttestationProcessingResult, tenant, owner, repo, issuer string) error {
	var want string

	if tenant == "" {
		want = fmt.Sprintf("https://github.com/%s", owner)
	} else {
		want = fmt.Sprintf("https://%s.ghe.com/%s", tenant, owner)
	}
	sourceRepositoryOwnerURI := attestation.VerificationResult.Signature.Certificate.Extensions.SourceRepositoryOwnerURI
	if !strings.EqualFold(want, sourceRepositoryOwnerURI) {
		return fmt.Errorf("expected SourceRepositoryOwnerURI to be %s, got %s", want, sourceRepositoryOwnerURI)
	}

	// if repo is set, check the SourceRepositoryURI field
	if repo != "" {
		if tenant == "" {
			want = fmt.Sprintf("https://github.com/%s", repo)
		} else {
			want = fmt.Sprintf("https://%s.ghe.com/%s", tenant, repo)
		}

		sourceRepositoryURI := attestation.VerificationResult.Signature.Certificate.Extensions.SourceRepositoryURI
		if !strings.EqualFold(want, sourceRepositoryURI) {
			return fmt.Errorf("expected SourceRepositoryURI to be %s, got %s", want, sourceRepositoryURI)
		}
	}

	// if issuer is anything other than the default, use the user-provided value;
	// otherwise, select the appropriate default based on the tenant
	if issuer != GitHubOIDCIssuer {
		want = issuer
	} else {
		if tenant != "" {
			want = fmt.Sprintf(GitHubTenantOIDCIssuer, tenant)
		} else {
			want = GitHubOIDCIssuer
		}
	}

	certIssuer := attestation.VerificationResult.Signature.Certificate.Extensions.Issuer
	if !strings.EqualFold(want, certIssuer) {
		if strings.Index(certIssuer, want+"/") == 0 {
			return fmt.Errorf("expected Issuer to be %s, got %s -- if you have a custom OIDC issuer policy for your enterprise, use the --cert-oidc-issuer flag with your expected issuer", want, certIssuer)
		} else {
			return fmt.Errorf("expected Issuer to be %s, got %s", want, certIssuer)
		}
	}

	return nil
}
