package verification

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/stretchr/testify/require"
)

// Note: Tests that require network access and TUF client initialization
// are in sigstore_integration_test.go with the //go:build integration tag.
// These unit tests focus on testing the logic without requiring network access.

// TestChooseVerifierWithNilPublicGood tests that chooseVerifier returns an error
// when a PGI attestation is encountered but the PGI verifier is nil (failed initialization).
func TestChooseVerifierWithNilPublicGood(t *testing.T) {
	verifier := &LiveSigstoreVerifier{
		Logger:       io.NewTestHandler(),
		NoPublicGood: false,
		PublicGood:   nil, // Simulate failed PGI initialization
		GitHub:       nil, // Not needed for this test
	}

	_, err := verifier.chooseVerifier(PublicGoodIssuerOrg)

	require.Error(t, err)
	require.ErrorContains(t, err, "public good verifier is not available")
}

// TestChooseVerifierUnrecognizedIssuer tests that an error is returned
// for unrecognized issuers.
func TestChooseVerifierUnrecognizedIssuer(t *testing.T) {
	verifier := &LiveSigstoreVerifier{
		Logger:       io.NewTestHandler(),
		NoPublicGood: false,
	}

	_, err := verifier.chooseVerifier("unknown-issuer")

	require.Error(t, err)
	require.ErrorContains(t, err, "leaf certificate issuer is not recognized")
}

func TestLiveSigstoreVerifier_noVerifierSet(t *testing.T) {
	verifier := &LiveSigstoreVerifier{
		Logger:       io.NewTestHandler(),
		NoPublicGood: true,
		PublicGood:   nil,
		GitHub:       nil,
	}

	require.True(t, verifier.noVerifierSet())
}
