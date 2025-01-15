package verification

import (
	"bufio"
	"bytes"
	"crypto/x509"
	"errors"
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

type SigstoreConfig struct {
	TrustedRoot  string
	Logger       *io.Handler
	NoPublicGood bool
	// If tenancy mode is not used, trust domain is empty
	TrustDomain string
}

type SigstoreVerifier interface {
	Verify(attestations []*api.Attestation, policy verify.PolicyBuilder) ([]*AttestationProcessingResult, error)
}

type LiveSigstoreVerifier struct {
	TrustedRoot  string
	Logger       *io.Handler
	NoPublicGood bool
	// If tenancy mode is not used, trust domain is empty
	TrustDomain string
}

var ErrNoAttestationsVerified = errors.New("no attestations were verified")

// NewLiveSigstoreVerifier creates a new LiveSigstoreVerifier struct
// that is used to verify artifacts and attestations against the
// Public Good, GitHub, or a custom trusted root.
func NewLiveSigstoreVerifier(config SigstoreConfig) *LiveSigstoreVerifier {
	return &LiveSigstoreVerifier{
		TrustedRoot:  config.TrustedRoot,
		Logger:       config.Logger,
		NoPublicGood: config.NoPublicGood,
		TrustDomain:  config.TrustDomain,
	}
}

func getBundleIssuer(b *bundle.Bundle) (string, error) {
	if !b.MinVersion("0.2") {
		return "", fmt.Errorf("unsupported bundle version: %s", b.MediaType)
	}
	verifyContent, err := b.VerificationContent()
	if err != nil {
		return "", fmt.Errorf("failed to get bundle verification content: %v", err)
	}
	leafCert := verifyContent.GetCertificate()
	if leafCert == nil {
		return "", fmt.Errorf("leaf cert not found")
	}
	if len(leafCert.Issuer.Organization) != 1 {
		return "", fmt.Errorf("expected the leaf certificate issuer to only have one organization")
	}
	return leafCert.Issuer.Organization[0], nil
}

func (v *LiveSigstoreVerifier) chooseVerifier(issuer string) (*verify.SignedEntityVerifier, error) {
	// if no custom trusted root is set, attempt to create a Public Good or
	// GitHub Sigstore verifier
	if v.TrustedRoot == "" {
		switch issuer {
		case PublicGoodIssuerOrg:
			if v.NoPublicGood {
				return nil, fmt.Errorf("detected public good instance but requested verification without public good instance")
			}
			return newPublicGoodVerifier()
		case GitHubIssuerOrg:
			return newGitHubVerifier(v.TrustDomain)
		default:
			return nil, fmt.Errorf("leaf certificate issuer is not recognized")
		}
	}

	customTrustRoots, err := os.ReadFile(v.TrustedRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to read file %s: %v", v.TrustedRoot, err)
	}

	reader := bufio.NewReader(bytes.NewReader(customTrustRoots))
	var line []byte
	var readError error
	line, readError = reader.ReadBytes('\n')
	for readError == nil {
		// Load each trusted root
		trustedRoot, err := root.NewTrustedRootFromJSON(line)
		if err != nil {
			return nil, fmt.Errorf("failed to create custom verifier: %v", err)
		}

		// Compare bundle leafCert issuer with trusted root cert authority
		certAuthorities := trustedRoot.FulcioCertificateAuthorities()
		for _, certAuthority := range certAuthorities {
			lowestCert, err := getLowestCertInChain(&certAuthority)
			if err != nil {
				return nil, err
			}

			// if the custom trusted root issuer is not set or doesn't match the given issuer, skip it
			if len(lowestCert.Issuer.Organization) == 0 || lowestCert.Issuer.Organization[0] != issuer {
				continue
			}

			// Determine what policy to use with this trusted root.
			//
			// Note that we are *only* inferring the policy with the
			// issuer. We *must* use the trusted root provided.
			switch issuer {
			case PublicGoodIssuerOrg:
				if v.NoPublicGood {
					return nil, fmt.Errorf("detected public good instance but requested verification without public good instance")
				}
				return newPublicGoodVerifierWithTrustedRoot(trustedRoot)
			case GitHubIssuerOrg:
				return newGitHubVerifierWithTrustedRoot(trustedRoot)
			default:
				// Make best guess at reasonable policy
				return newCustomVerifier(trustedRoot)
			}
		}
		line, readError = reader.ReadBytes('\n')
	}

	return nil, fmt.Errorf("unable to use provided trusted roots")
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

func (v *LiveSigstoreVerifier) verify(attestation *api.Attestation, policy verify.PolicyBuilder) (*AttestationProcessingResult, error) {
	issuer, err := getBundleIssuer(attestation.Bundle)
	if err != nil {
		return nil, fmt.Errorf("failed to get bundle issuer: %v", err)
	}

	// determine which verifier should attempt verification against the bundle
	verifier, err := v.chooseVerifier(issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to find recognized issuer from bundle content: %v", err)
	}

	v.Logger.VerbosePrintf("Attempting verification against issuer \"%s\"\n", issuer)
	// attempt to verify the attestation
	result, err := verifier.Verify(attestation.Bundle, policy)
	// if verification fails, create the error and exit verification early
	if err != nil {
		v.Logger.VerbosePrint(v.Logger.ColorScheme.Redf(
			"Failed to verify against issuer \"%s\" \n\n", issuer,
		))

		return nil, fmt.Errorf("verifying with issuer \"%s\"", issuer)
	}

	// if verification is successful, add the result
	// to the AttestationProcessingResult entry
	v.Logger.VerbosePrint(v.Logger.ColorScheme.Greenf(
		"SUCCESS - attestation signature verified with \"%s\"\n", issuer,
	))

	return &AttestationProcessingResult{
		Attestation:        attestation,
		VerificationResult: result,
	}, nil
}

func (v *LiveSigstoreVerifier) Verify(attestations []*api.Attestation, policy verify.PolicyBuilder) ([]*AttestationProcessingResult, error) {
	if len(attestations) == 0 {
		return nil, ErrNoAttestationsVerified
	}

	results := make([]*AttestationProcessingResult, len(attestations))
	var verifyCount int
	var lastError error
	totalAttestations := len(attestations)
	for i, a := range attestations {
		v.Logger.VerbosePrintf("Verifying attestation %d/%d against the configured Sigstore trust roots\n", i+1, totalAttestations)

		apr, err := v.verify(a, policy)
		if err != nil {
			lastError = err
			// move onto the next attestation in the for loop if verification fails
			continue
		}
		// otherwise, add the result to the results slice and increment verifyCount
		results[verifyCount] = apr
		verifyCount++
	}

	if verifyCount == 0 {
		return nil, lastError
	}

	// truncate the results slice to only include verified attestations
	results = results[:verifyCount]

	return results, nil
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

func newGitHubVerifier(trustDomain string) (*verify.SignedEntityVerifier, error) {
	var tr string

	opts := GitHubTUFOptions()
	client, err := tuf.New(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUF client: %v", err)
	}

	if trustDomain == "" {
		tr = "trusted_root.json"
	} else {
		tr = fmt.Sprintf("%s.trusted_root.json", trustDomain)
	}
	jsonBytes, err := client.GetTarget(tr)
	if err != nil {
		return nil, err
	}
	trustedRoot, err := root.NewTrustedRootFromJSON(jsonBytes)
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
