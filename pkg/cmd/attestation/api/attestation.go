package api

import (
	"errors"
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
