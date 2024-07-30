package verification

import (
	"fmt"
	"strings"
)

func VerifyCertExtensions(results []*AttestationProcessingResult, owner string, repo string) error {
	for _, attestation := range results {
		if owner != "" {
			expectedSourceRepositoryOwnerURI := fmt.Sprintf("https://github.com/%s", owner)
			sourceRepositoryOwnerURI := attestation.VerificationResult.Signature.Certificate.Extensions.SourceRepositoryOwnerURI
			if !strings.EqualFold(expectedSourceRepositoryOwnerURI, sourceRepositoryOwnerURI) {
				return fmt.Errorf("expected SourceRepositoryOwnerURI to be %s, got %s", expectedSourceRepositoryOwnerURI, sourceRepositoryOwnerURI)
			}
		}

		if repo != "" {
			expectedSourceRepositoryURI := fmt.Sprintf("https://github.com/%s", repo)
			sourceRepositoryURI := attestation.VerificationResult.Signature.Certificate.Extensions.SourceRepositoryURI
			if !strings.EqualFold(expectedSourceRepositoryURI, sourceRepositoryURI) {
				return fmt.Errorf("expected SourceRepositoryURI to be %s, got %s", expectedSourceRepositoryURI, sourceRepositoryURI)
			}
		}
	}
	return nil
}
