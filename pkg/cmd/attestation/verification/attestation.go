package verification

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

var ErrUnrecognisedBundleExtension = errors.New("bundle file extension not supported, must be json or jsonl")

type FetchAttestationsConfig struct {
	APIClient  api.Client
	BundlePath string
	Digest     string
	Limit      int
	Owner      string
	Repo       string
}

func (c *FetchAttestationsConfig) IsBundleProvided() bool {
	return c.BundlePath != ""
}

func GetAttestations(c FetchAttestationsConfig) ([]*api.Attestation, error) {
	if c.IsBundleProvided() {
		return GetLocalAttestations(c.BundlePath)
	}
	return GetRemoteAttestations(c)
}

// GetLocalAttestations returns a slice of attestations read from a local bundle file.
func GetLocalAttestations(path string) ([]*api.Attestation, error) {
	fileExt := filepath.Ext(path)
	switch fileExt {
	case ".json":
		attestations, err := loadBundleFromJSONFile(path)
		if err != nil {
			return nil, fmt.Errorf("bundle could not be loaded from JSON file: %v", err)
		}
		return attestations, nil
	case ".jsonl":
		attestations, err := loadBundlesFromJSONLinesFile(path)
		if err != nil {
			return nil, fmt.Errorf("bundles could not be loaded from JSON lines file: %v", err)
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
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open file: %v", err)
	}
	defer file.Close()

	attestations := []*api.Attestation{}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		b := scanner.Bytes()
		var bundle bundle.ProtobufBundle
		bundle.Bundle = new(protobundle.Bundle)
		err = bundle.UnmarshalJSON(b)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal bundle from JSON: %v", err)
		}
		a := api.Attestation{Bundle: &bundle}
		attestations = append(attestations, &a)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return attestations, nil
}

func GetRemoteAttestations(c FetchAttestationsConfig) ([]*api.Attestation, error) {
	if c.APIClient == nil {
		return nil, fmt.Errorf("api client must be provided")
	}
	// check if Repo is set first because if Repo has been set, Owner will be set using the value of Repo.
	// If Repo is not set, the field will remain empty. It will not be populated using the value of Owner.
	if c.Repo != "" {
		attestations, err := c.APIClient.GetByRepoAndDigest(c.Repo, c.Digest, c.Limit)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch attestations from %s: %w", c.Repo, err)
		}
		return attestations, nil
	} else if c.Owner != "" {
		attestations, err := c.APIClient.GetByOwnerAndDigest(c.Owner, c.Digest, c.Limit)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch attestations from %s: %w", c.Owner, err)
		}
		return attestations, nil
	}
	return nil, fmt.Errorf("owner or repo must be provided")
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
