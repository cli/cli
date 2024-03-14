package download

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
)

type MetadataStore struct {
	outputPath string
}

func (s *MetadataStore) createJSONLinesFilePath(artifact string) string {
	path := fmt.Sprintf("%s.jsonl", artifact)
	if s.outputPath != "" {
		return fmt.Sprintf("%s/%s", s.outputPath, path)
	}
	return path
}

func (s *MetadataStore) createMetadataFile(artifactDigest string, attestationsResp []*api.Attestation) (string, error) {
	metadataFilePath := s.createJSONLinesFilePath(artifactDigest)

	f, err := os.Create(metadataFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create trusted metadata file: %w", err)
	}

	for _, resp := range attestationsResp {
		bundle := resp.Bundle
		attBytes, err := json.Marshal(bundle)
		if err != nil {
			return "", fmt.Errorf("failed to marshall attestation to JSON: %w", err)
		}

		withNewline := fmt.Sprintf("%s\n", attBytes)
		_, err = f.Write([]byte(withNewline))
		if err != nil {
			if err = f.Close(); err != nil {
				return "", fmt.Errorf("failed to close file while handling write error: %w", err)
			}

			return "", fmt.Errorf("failed to write trusted metadata: %w", err)
		}
	}

	if err = f.Close(); err != nil {
		return "", fmt.Errorf("failed to close file after writing metadata: %w", err)
	}

	return metadataFilePath, nil
}
