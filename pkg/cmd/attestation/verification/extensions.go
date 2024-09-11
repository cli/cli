package verification

import (
	"errors"
	"fmt"
	"strings"
)

func VerifyCertExtensions(results []*AttestationProcessingResult, tenant, owner, repo string) error {
	if len(results) == 0 {
		return errors.New("no attestations proccessing results")
	}

	for _, attestation := range results {
		if err := verifyCertExtensions(attestation, tenant, owner, repo); err != nil {
			return err
		}
	}
	return nil
}

func verifyCertExtensions(attestation *AttestationProcessingResult, tenant, owner, repo string) error {
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

	return nil
}
