package api

const MetaPath = "meta"

type ArtifactAttestations struct {
	TrustDomain string `json:"trust_domain"`
}

type Domain struct {
	ArtifactAttestations ArtifactAttestations `json:"artifact_attestations"`
}

type MetaResponse struct {
	Domains Domain `json:"domains"`
}
