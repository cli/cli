package inspect

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
)

type workflow struct {
	Repository string `json:"repository"`
}

type externalParameters struct {
	Workflow workflow `json:"workflow"`
}

type githubInfo struct {
	RepositoryID      string `json:"repository_id"`
	RepositoryOwnerId string `json:"repository_owner_id"`
}

type internalParameters struct {
	GitHub githubInfo `json:"github"`
}

type buildDefinition struct {
	ExternalParameters externalParameters `json:"externalParameters"`
	InternalParameters internalParameters `json:"internalParameters"`
}

type metadata struct {
	InvocationID string `json:"invocationId"`
}

type runDetails struct {
	Metadata metadata `json:"metadata"`
}

// Predicate captures the predicate of a given attestation
type Predicate struct {
	BuildDefinition buildDefinition `json:"buildDefinition"`
	RunDetails      runDetails      `json:"runDetails"`
}

// AttestationDetail captures attestation source details
// that will be returned by the inspect command
type AttestationDetail struct {
	OrgName        string `json:"orgName"`
	OrgID          string `json:"orgId"`
	RepositoryName string `json:"repositoryName"`
	RepositoryID   string `json:"repositoryId"`
	WorkflowID     string `json:"workflowId"`
}

func getOrgAndRepo(repoURL string) (string, string, error) {
	after, found := strings.CutPrefix(repoURL, "https://github.com/")
	if !found {
		return "", "", fmt.Errorf("failed to get org and repo from %s", repoURL)
	}

	parts := strings.Split(after, "/")
	return parts[0], parts[1], nil
}

func getAttestationDetail(attr api.Attestation) (AttestationDetail, error) {
	envelope, err := attr.Bundle.Envelope()
	if err != nil {
		return AttestationDetail{}, fmt.Errorf("failed to get envelope from bundle: %v", err)
	}

	statement, err := envelope.EnvelopeContent().Statement()
	if err != nil {
		return AttestationDetail{}, fmt.Errorf("failed to get statement from envelope: %v", err)
	}

	var predicate Predicate
	predicateJson, err := json.Marshal(statement.Predicate)
	if err != nil {
		return AttestationDetail{}, fmt.Errorf("failed to marshal predicate: %v", err)
	}

	err = json.Unmarshal(predicateJson, &predicate)
	if err != nil {
		return AttestationDetail{}, fmt.Errorf("failed to unmarshal predicate: %v", err)
	}

	org, repo, err := getOrgAndRepo(predicate.BuildDefinition.ExternalParameters.Workflow.Repository)
	if err != nil {
		return AttestationDetail{}, fmt.Errorf("failed to parse attestation content: %v", err)
	}

	return AttestationDetail{
		OrgName:        org,
		OrgID:          predicate.BuildDefinition.InternalParameters.GitHub.RepositoryOwnerId,
		RepositoryName: repo,
		RepositoryID:   predicate.BuildDefinition.InternalParameters.GitHub.RepositoryID,
		WorkflowID:     predicate.RunDetails.Metadata.InvocationID,
	}, nil
}

func getDetailsAsSlice(results []*verification.AttestationProcessingResult) ([][]string, error) {
	details := make([][]string, len(results))

	for i, result := range results {
		detail, err := getAttestationDetail(*result.Attestation)
		if err != nil {
			return nil, fmt.Errorf("failed to get attestation detail: %v", err)
		}
		details[i] = []string{detail.RepositoryName, detail.RepositoryID, detail.OrgName, detail.OrgID, detail.WorkflowID}
	}
	return details, nil
}

func getAttestationDetails(results []*verification.AttestationProcessingResult) ([]AttestationDetail, error) {
	details := make([]AttestationDetail, len(results))

	for i, result := range results {
		detail, err := getAttestationDetail(*result.Attestation)
		if err != nil {
			return nil, fmt.Errorf("failed to get attestation detail: %v", err)
		}
		details[i] = detail
	}
	return details, nil
}
