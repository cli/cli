package verification

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
)

var (
	GitHubOIDCIssuer       = "https://token.actions.githubusercontent.com"
	GitHubTenantOIDCIssuer = "https://token.actions.%s.ghe.com"
)

// VerifyCertExtensions allows us to perform case insensitive comparisons of certificate extensions
func VerifyCertExtensions(results []*AttestationProcessingResult, ec EnforcementCriteria) ([]*AttestationProcessingResult, error) {
	if len(results) == 0 {
		return nil, errors.New("no attestations processing results")
	}

	verified := make([]*AttestationProcessingResult, 0, len(results))
	var lastErr error
	for _, attestation := range results {
		if err := verifyCertExtensions(*attestation.VerificationResult.Signature.Certificate, ec.Certificate); err != nil {
			lastErr = err
			// move onto the next attestation in the for loop if verification fails
			continue
		}
		// otherwise, add the result to the results slice and increment verifyCount
		verified = append(verified, attestation)
	}

	// if we have exited the for loop without verifying any attestations,
	// return the last error found
	if len(verified) == 0 {
		return nil, lastErr
	}

	return verified, nil
}

func verifyCertExtensions(given, expected certificate.Summary) error {
	if !strings.EqualFold(expected.SourceRepositoryOwnerURI, given.SourceRepositoryOwnerURI) {
		return fmt.Errorf("expected SourceRepositoryOwnerURI to be %s, got %s", expected.SourceRepositoryOwnerURI, given.SourceRepositoryOwnerURI)
	}

	// if repo is set, compare the SourceRepositoryURI fields
	if expected.SourceRepositoryURI != "" && !strings.EqualFold(expected.SourceRepositoryURI, given.SourceRepositoryURI) {
		return fmt.Errorf("expected SourceRepositoryURI to be %s, got %s", expected.SourceRepositoryURI, given.SourceRepositoryURI)
	}

	// compare the OIDC issuers. If not equal, return an error depending
	// on if there is a partial match
	if !strings.EqualFold(expected.Issuer, given.Issuer) {
		if strings.Index(given.Issuer, expected.Issuer+"/") == 0 {
			return fmt.Errorf("expected Issuer to be %s, got %s -- if you have a custom OIDC issuer policy for your enterprise, use the --cert-oidc-issuer flag with your expected issuer", expected.Issuer, given.Issuer)
		}
		return fmt.Errorf("expected Issuer to be %s, got %s", expected.Issuer, given.Issuer)
	}

	return nil
}
