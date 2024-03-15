package download

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
)

var ErrAttestationFileCreation = fmt.Errorf("failed to write attestations to file")

type MetadataStore interface {
	createMetadataFile(artifactDigest string, attestationsResp []*api.Attestation) (string, error)
}

type LiveStore struct {
	outputPath string
}

func (s *LiveStore) createJSONLinesFilePath(artifact string) string {
	path := fmt.Sprintf("%s.jsonl", artifact)
	if s.outputPath != "" {
		return fmt.Sprintf("%s/%s", s.outputPath, path)
	}
	return path
}

func (s *LiveStore) createMetadataFile(artifactDigest string, attestationsResp []*api.Attestation) (string, error) {
	metadataFilePath := s.createJSONLinesFilePath(artifactDigest)

	f, err := os.Create(metadataFilePath)
	if err != nil {
		return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to create file: %w", err))
	}

	for _, resp := range attestationsResp {
		bundle := resp.Bundle
		attBytes, err := json.Marshal(bundle)
		if err != nil {
			return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to marshall attestation to JSON while writing to file: %w", err))
		}

		withNewline := fmt.Sprintf("%s\n", attBytes)
		_, err = f.Write([]byte(withNewline))
		if err != nil {
			if err = f.Close(); err != nil {
				return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to close file while handling write error: %w", err))
			}

			return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to write attestations: %w", err))
		}
	}

	if err = f.Close(); err != nil {
		return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to close file after writing attestations: %w", err))
	}

	return metadataFilePath, nil
}
