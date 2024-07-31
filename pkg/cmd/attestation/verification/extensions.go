package verification

import (
	"fmt"
	"strings"
)

func VerifyCertExtensions(results []*AttestationProcessingResult, owner string, repo string) error {
	for _, attestation := range results {
		// TODO: handle proxima prefix
		expectedSourceRepositoryOwnerURI := fmt.Sprintf("https://github.com/%s", owner)
		sourceRepositoryOwnerURI := attestation.VerificationResult.Signature.Certificate.Extensions.SourceRepositoryOwnerURI
		if !strings.EqualFold(expectedSourceRepositoryOwnerURI, sourceRepositoryOwnerURI) {
			return fmt.Errorf("expected SourceRepositoryOwnerURI to be %s, got %s", expectedSourceRepositoryOwnerURI, sourceRepositoryOwnerURI)
		}

		// if repo is set, check the SourceRepositoryURI field
		if repo != "" {
			// TODO: handle proxima prefix
			expectedSourceRepositoryURI := fmt.Sprintf("https://github.com/%s", repo)
			sourceRepositoryURI := attestation.VerificationResult.Signature.Certificate.Extensions.SourceRepositoryURI
			if !strings.EqualFold(expectedSourceRepositoryURI, sourceRepositoryURI) {
				return fmt.Errorf("expected SourceRepositoryURI to be %s, got %s", expectedSourceRepositoryURI, sourceRepositoryURI)
			}
		}
	}
	return nil
}
