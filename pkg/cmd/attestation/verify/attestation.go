package verify

import (
	"errors"
	"fmt"

	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

func getAttestations(o *Options, a artifact.DigestedArtifact) ([]*api.Attestation, string, error) {
	// Fetch attestations from GitHub API within this if block since predicate type
	// filter is done when the API is called
	if o.FetchAttestationsFromGitHubAPI() {
		if o.APIClient == nil {
			errMsg := "✗ No APIClient provided"
			return nil, errMsg, errors.New(errMsg)
		}

		params := api.FetchParams{
			Digest:        a.DigestWithAlg(),
			Limit:         o.Limit,
			Owner:         o.Owner,
			PredicateType: o.PredicateType,
			Repo:          o.Repo,
		}

		attestations, err := o.APIClient.GetByDigest(params)
		if err != nil {
			msg := "✗ Loading attestations from GitHub API failed"
			return nil, msg, err
		}
		pluralAttestation := text.Pluralize(len(attestations), "attestation")
		msg := fmt.Sprintf("Loaded %s from GitHub API", pluralAttestation)
		return attestations, msg, nil
	}

	// Fetch attestations from local bundle or OCI registry
	// Predicate type filtering is done after the attestations are fetched
	var attestations []*api.Attestation
	var err error
	var msg string
	if o.BundlePath != "" {
		attestations, err = verification.GetLocalAttestations(o.BundlePath)
		if err != nil {
			pluralAttestation := text.Pluralize(len(attestations), "attestation")
			msg = fmt.Sprintf("Loaded %s from %s", pluralAttestation, o.BundlePath)
		} else {
			msg = fmt.Sprintf("Loaded %d attestations from %s", len(attestations), o.BundlePath)
		}
	} else if o.UseBundleFromRegistry {
		attestations, err = verification.GetOCIAttestations(o.OCIClient, a)
		if err != nil {
			msg = "✗ Loading attestations from OCI registry failed"
		} else {
			pluralAttestation := text.Pluralize(len(attestations), "attestation")
			msg = fmt.Sprintf("Loaded %s from OCI registry", pluralAttestation)
		}
	}
	if err != nil {
		return nil, msg, err
	}

	filtered, err := api.FilterAttestations(o.PredicateType, attestations)
	if err != nil {
		return nil, err.Error(), err
	}
	return filtered, msg, nil
}

func verifyAttestations(art artifact.DigestedArtifact, att []*api.Attestation, sgVerifier verification.SigstoreVerifier, ec verification.EnforcementCriteria) ([]*verification.AttestationProcessingResult, string, error) {
	sgPolicy, err := buildSigstoreVerifyPolicy(ec, art)
	if err != nil {
		logMsg := "✗ Failed to build Sigstore verification policy"
		return nil, logMsg, err
	}

	sigstoreVerified, err := sgVerifier.Verify(att, sgPolicy)
	if err != nil {
		logMsg := "✗ Sigstore verification failed"
		return nil, logMsg, err
	}

	// Verify extensions
	certExtVerified, err := verification.VerifyCertExtensions(sigstoreVerified, ec)
	if err != nil {
		logMsg := "✗ Policy verification failed"
		return nil, logMsg, err
	}

	return certExtVerified, "", nil
}
