package verification

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

const SLSAPredicateV1 = "https://slsa.dev/provenance/v1"

var ErrUnrecognisedBundleExtension = errors.New("bundle file extension not supported, must be json or jsonl")
var ErrEmptyBundleFile = errors.New("provided bundle file is empty")

type FetchRemoteAttestationsParams struct {
	Digest string
	Limit  int
	Owner  string
	Repo   string
}

// GetLocalAttestations returns a slice of attestations read from a local bundle file.
func GetLocalAttestations(path string) ([]*api.Attestation, error) {
	fileExt := filepath.Ext(path)
	switch fileExt {
	case ".json":
		attestations, err := loadBundleFromJSONFile(path)
		if err != nil {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				return nil, fmt.Errorf("bundle could not be loaded from JSON file at %s", path)
			} else if errors.Is(err, bundle.ErrValidation) {
				return nil, err
			}
			return nil, fmt.Errorf("bundle content could not be parsed")
		}
		return attestations, nil
	case ".jsonl":
		attestations, err := loadBundlesFromJSONLinesFile(path)
		if err != nil {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				return nil, fmt.Errorf("bundles could not be loaded from JSON lines file at %s", path)
			} else if errors.Is(err, bundle.ErrValidation) {
				return nil, err
			}
			return nil, fmt.Errorf("bundle content could not be parsed")
		}
		return attestations, nil
	}
	return nil, ErrUnrecognisedBundleExtension
}

func loadBundleFromJSONFile(path string) ([]*api.Attestation, error) {
	localAttestation, err := bundle.LoadJSONFromPath(path)
	if err != nil {
		return nil, err
	}

	return []*api.Attestation{{Bundle: localAttestation}}, nil
}

func loadBundlesFromJSONLinesFile(path string) ([]*api.Attestation, error) {
	fileContent, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	attestations := []*api.Attestation{}

	decoder := json.NewDecoder(bytes.NewReader(fileContent))

	for decoder.More() {
		var b bundle.Bundle
		b.Bundle = new(protobundle.Bundle)
		if err := decoder.Decode(&b); err != nil {
			return nil, err
		}
		a := api.Attestation{Bundle: &b}
		attestations = append(attestations, &a)
	}

	if len(attestations) == 0 {
		return nil, ErrEmptyBundleFile
	}

	return attestations, nil
}

func GetRemoteAttestations(client api.Client, params FetchRemoteAttestationsParams) ([]*api.Attestation, error) {
	if client == nil {
		return nil, fmt.Errorf("api client must be provided")
	}
	// check if Repo is set first because if Repo has been set, Owner will be set using the value of Repo.
	// If Repo is not set, the field will remain empty. It will not be populated using the value of Owner.
	if params.Repo != "" {
		attestations, err := client.GetByRepoAndDigest(params.Repo, params.Digest, params.Limit)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch attestations from %s: %w", params.Repo, err)
		}
		return attestations, nil
	} else if params.Owner != "" {
		attestations, err := client.GetByOwnerAndDigest(params.Owner, params.Digest, params.Limit)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch attestations from %s: %w", params.Owner, err)
		}
		return attestations, nil
	}
	return nil, fmt.Errorf("owner or repo must be provided")
}

func GetOCIAttestations(client oci.Client, artifact artifact.DigestedArtifact) ([]*api.Attestation, error) {
	attestations, err := client.GetAttestations(artifact.NameRef(), artifact.DigestWithAlg())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OCI attestations: %w", err)
	}
	if len(attestations) == 0 {
		return nil, fmt.Errorf("no attestations found in the OCI registry. Retry the command without the --bundle-from-oci flag to check GitHub for the attestation")
	}
	return attestations, nil
}

type IntotoStatement struct {
	PredicateType string `json:"predicateType"`
}

func FilterAttestations(predicateType string, attestations []*api.Attestation) []*api.Attestation {
	filteredAttestations := []*api.Attestation{}

	for _, each := range attestations {
		dsseEnvelope := each.Bundle.GetDsseEnvelope()
		if dsseEnvelope != nil {
			if dsseEnvelope.PayloadType != "application/vnd.in-toto+json" {
				// Don't fail just because an entry isn't intoto
				continue
			}
			var intotoStatement IntotoStatement
			if err := json.Unmarshal([]byte(dsseEnvelope.Payload), &intotoStatement); err != nil {
				// Don't fail just because a single entry can't be unmarshalled
				continue
			}
			if intotoStatement.PredicateType == predicateType {
				filteredAttestations = append(filteredAttestations, each)
			}
		}
	}

	return filteredAttestations
}
