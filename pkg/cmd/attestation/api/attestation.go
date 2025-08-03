package api

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sigstore/sigstore-go/pkg/bundle"
)

const (
	GetAttestationByRepoAndSubjectDigestPath  = "repos/%s/attestations/%s"
	GetAttestationByOwnerAndSubjectDigestPath = "orgs/%s/attestations/%s"
)

var ErrNoAttestationsFound = errors.New("no attestations found")

type Attestation struct {
	Bundle    *bundle.Bundle `json:"bundle"`
	BundleURL string         `json:"bundle_url"`
}

type AttestationsResponse struct {
	Attestations []*Attestation `json:"attestations"`
}

type IntotoStatement struct {
	PredicateType string `json:"predicateType"`
}

func FilterAttestations(predicateType string, attestations []*Attestation) ([]*Attestation, error) {
	filteredAttestations := []*Attestation{}

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

	if len(filteredAttestations) == 0 {
		return nil, fmt.Errorf("no attestations found with predicate type: %s", predicateType)
	}

	return filteredAttestations, nil
}
