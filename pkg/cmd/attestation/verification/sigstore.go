package verification

import (
	"bufio"
	"bytes"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const (
	PublicGoodIssuerOrg = "sigstore.dev"
	GitHubIssuerOrg     = "GitHub, Inc."
)

// AttestationProcessingResult captures processing a given attestation's signature verification and policy evaluation
type AttestationProcessingResult struct {
	Attestation        *api.Attestation           `json:"attestation"`
	VerificationResult *verify.VerificationResult `json:"verificationResult"`
}

type SigstoreResults struct {
	VerifyResults []*AttestationProcessingResult
	Error         error
}

type SigstoreConfig struct {
	TrustedRoot  string
	Logger       *io.Handler
	NoPublicGood bool
}

type SigstoreVerifier interface {
	Verify(attestations []*api.Attestation, policy verify.PolicyBuilder) *SigstoreResults
}

type LiveSigstoreVerifier struct {
	config SigstoreConfig
}

// NewLiveSigstoreVerifier creates a new LiveSigstoreVerifier struct
// that is used to verify artifacts and attestations against the
// Public Good, GitHub, or a custom trusted root.
func NewLiveSigstoreVerifier(config SigstoreConfig) *LiveSigstoreVerifier {
	return &LiveSigstoreVerifier{
		config: config,
	}
}

func (v *LiveSigstoreVerifier) chooseVerifier(b *bundle.ProtobufBundle) (*verify.SignedEntityVerifier, string, error) {
	if !b.MinVersion("0.2") {
		return nil, "", fmt.Errorf("unsupported bundle version: %s", b.MediaType)
	}
	verifyContent, err := b.VerificationContent()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get bundle verification content: %v", err)
	}
	leafCert := verifyContent.GetCertificate()
	if leafCert == nil {
		return nil, "", fmt.Errorf("leaf cert not found")
	}
	if len(leafCert.Issuer.Organization) != 1 {
		return nil, "", fmt.Errorf("expected the leaf certificate issuer to only have one organization")
	}
	issuer := leafCert.Issuer.Organization[0]

	if v.config.TrustedRoot != "" {
		customTrustRoots, err := os.ReadFile(v.config.TrustedRoot)
		if err != nil {
			return nil, "", fmt.Errorf("unable to read file %s: %v", v.config.TrustedRoot, err)
		}

		reader := bufio.NewReader(bytes.NewReader(customTrustRoots))
		var line []byte
		var readError error
		line, readError = reader.ReadBytes('\n')
		for readError == nil {
			// Load each trusted root
			trustedRoot, err := root.NewTrustedRootFromJSON(line)
			if err != nil {
				return nil, "", fmt.Errorf("failed to create custom verifier: %v", err)
			}

			// Compare bundle leafCert issuer with trusted root cert authority
			certAuthorities := trustedRoot.FulcioCertificateAuthorities()
			for _, certAuthority := range certAuthorities {
				lowestCert, err := getLowestCertInChain(&certAuthority)
				if err != nil {
					return nil, "", err
				}

				if len(lowestCert.Issuer.Organization) == 0 {
					continue
				}

				if lowestCert.Issuer.Organization[0] == issuer {
					// Determine what policy to use with this trusted root.
					//
					// Note that we are *only* inferring the policy with the
					// issuer. We *must* use the trusted root provided.
					if issuer == PublicGoodIssuerOrg {
						if v.config.NoPublicGood {
							return nil, "", fmt.Errorf("Detected public good instance but requested verification without public good instance")
						}
						verifier, err := newPublicGoodVerifierWithTrustedRoot(trustedRoot)
						if err != nil {
							return nil, "", err
						}
						return verifier, issuer, nil
					} else if issuer == GitHubIssuerOrg {
						verifier, err := newGitHubVerifierWithTrustedRoot(trustedRoot)
						if err != nil {
							return nil, "", err
						}
						return verifier, issuer, nil
					} else {
						// Make best guess at reasonable policy
						customVerifier, err := newCustomVerifier(trustedRoot)
						if err != nil {
							return nil, "", fmt.Errorf("failed to create custom verifier: %v", err)
						}
						return customVerifier, issuer, nil
					}
				}
			}
			line, readError = reader.ReadBytes('\n')
		}
		return nil, "", fmt.Errorf("unable to use provided trusted roots")
	}

	if leafCert.Issuer.Organization[0] == PublicGoodIssuerOrg && !v.config.NoPublicGood {
		publicGoodVerifier, err := newPublicGoodVerifier()
		if err != nil {
			return nil, "", fmt.Errorf("failed to create Public Good Sigstore verifier: %v", err)
		}

		return publicGoodVerifier, issuer, nil
	} else if leafCert.Issuer.Organization[0] == GitHubIssuerOrg || v.config.NoPublicGood {
		ghVerifier, err := newGitHubVerifier()
		if err != nil {
			return nil, "", fmt.Errorf("failed to create GitHub Sigstore verifier: %v", err)
		}

		return ghVerifier, issuer, nil
	}

	return nil, "", fmt.Errorf("leaf certificate issuer is not recognized")
}

func getLowestCertInChain(ca *root.CertificateAuthority) (*x509.Certificate, error) {
	if ca.Leaf != nil {
		return ca.Leaf, nil
	} else if len(ca.Intermediates) > 0 {
		return ca.Intermediates[0], nil
	} else if ca.Root != nil {
		return ca.Root, nil
	}

	return nil, fmt.Errorf("certificate authority had no certificates")
}

func (v *LiveSigstoreVerifier) Verify(attestations []*api.Attestation, policy verify.PolicyBuilder) *SigstoreResults {
	// initialize the processing results before attempting to verify
	// with multiple verifiers
	results := make([]*AttestationProcessingResult, len(attestations))
	for i, att := range attestations {
		apr := &AttestationProcessingResult{
			Attestation: att,
		}
		results[i] = apr
	}

	totalAttestations := len(attestations)
	for i, apr := range results {
		v.config.Logger.VerbosePrintf("Verifying attestation %d/%d against the configured Sigstore trust roots\n", i+1, totalAttestations)

		// determine which verifier should attempt verification against the bundle
		verifier, issuer, err := v.chooseVerifier(apr.Attestation.Bundle)
		if err != nil {
			return &SigstoreResults{
				Error: fmt.Errorf("failed to find recognized issuer from bundle content: %v", err),
			}
		}

		v.config.Logger.VerbosePrintf("Attempting verification against issuer \"%s\"\n", issuer)
		// attempt to verify the attestation
		result, err := verifier.Verify(apr.Attestation.Bundle, policy)
		// if verification fails, create the error and exit verification early
		if err != nil {
			v.config.Logger.VerbosePrint(v.config.Logger.ColorScheme.Redf(
				"Failed to verify against issuer \"%s\" \n\n", issuer,
			))

			return &SigstoreResults{
				Error: fmt.Errorf("verifying with issuer \"%s\": %v", issuer, err),
			}
		}

		// if verification is successful, add the result
		// to the AttestationProcessingResult entry
		v.config.Logger.VerbosePrint(v.config.Logger.ColorScheme.Greenf(
			"SUCCESS - attestation signature verified with \"%s\"\n", issuer,
		))
		apr.VerificationResult = result
	}

	return &SigstoreResults{
		VerifyResults: results,
	}
}

func newCustomVerifier(trustedRoot *root.TrustedRoot) (*verify.SignedEntityVerifier, error) {
	// All we know about this trust root is its configuration so make some
	// educated guesses as to what the policy should be.
	verifierConfig := []verify.VerifierOption{}
	// This requires some independent corroboration of the signing certificate
	// (e.g. from Sigstore Fulcio) time, one of:
	// - a signed timestamp from a timestamp authority in the trusted root
	// - a transparency log entry (e.g. from Sigstore Rekor)
	verifierConfig = append(verifierConfig, verify.WithObserverTimestamps(1))

	// Infer verification options from contents of trusted root
	if len(trustedRoot.RekorLogs()) > 0 {
		verifierConfig = append(verifierConfig, verify.WithTransparencyLog(1))
	}

	gv, err := verify.NewSignedEntityVerifier(trustedRoot, verifierConfig...)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom verifier: %v", err)
	}

	return gv, nil
}

func newGitHubVerifier() (*verify.SignedEntityVerifier, error) {
	opts := GitHubTUFOptions()
	client, err := tuf.New(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUF client: %v", err)
	}
	trustedRoot, err := root.GetTrustedRoot(client)
	if err != nil {
		return nil, err
	}
	return newGitHubVerifierWithTrustedRoot(trustedRoot)
}

func newGitHubVerifierWithTrustedRoot(trustedRoot *root.TrustedRoot) (*verify.SignedEntityVerifier, error) {
	gv, err := verify.NewSignedEntityVerifier(trustedRoot, verify.WithSignedTimestamps(1))
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub verifier: %v", err)
	}

	return gv, nil
}

func newPublicGoodVerifier() (*verify.SignedEntityVerifier, error) {
	opts := DefaultOptionsWithCacheSetting()
	client, err := tuf.New(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUF client: %v", err)
	}
	trustedRoot, err := root.GetTrustedRoot(client)
	if err != nil {
		return nil, fmt.Errorf("failed to get trusted root: %v", err)
	}

	return newPublicGoodVerifierWithTrustedRoot(trustedRoot)
}

func newPublicGoodVerifierWithTrustedRoot(trustedRoot *root.TrustedRoot) (*verify.SignedEntityVerifier, error) {
	sv, err := verify.NewSignedEntityVerifier(trustedRoot, verify.WithSignedCertificateTimestamps(1), verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1))
	if err != nil {
		return nil, fmt.Errorf("failed to create Public Good verifier: %v", err)
	}

	return sv, nil
}
