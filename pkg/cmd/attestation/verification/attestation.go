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

// GetLocalAttestations returns a slice of attestations read from a local bundle file.
func GetLocalAttestations(path string) ([]*api.Attestation, error) {
	var attestations []*api.Attestation
	var err error
	fileExt := filepath.Ext(path)
	if fileExt == ".json" {
		attestations, err = loadBundleFromJSONFile(path)
	} else if fileExt == ".jsonl" {
		attestations, err = loadBundlesFromJSONLinesFile(path)
	} else {
		return nil, ErrUnrecognisedBundleExtension
	}

	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return nil, fmt.Errorf("could not load content from file path %s: %w", path, err)
		} else if errors.Is(err, bundle.ErrValidation) {
			return nil, err
		}
		return nil, fmt.Errorf("bundle content could not be parsed: %w", err)
	}

	return attestations, nil
}

func loadBundleFromJSONFile(path string) ([]*api.Attestation, error) {
	b, err := bundle.LoadJSONFromPath(path)
	if err != nil {
		return nil, err
	}

	return []*api.Attestation{{Bundle: b}}, nil
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
