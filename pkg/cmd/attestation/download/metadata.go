package download

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

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
	if runtime.GOOS == "windows" {
		// Colons are special characters in Windows and cannot be used in file names.
		// Replace them with dashes to avoid issues.
		artifact = strings.ReplaceAll(artifact, ":", "-")
	}

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
		return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to create file: %v", err))
	}

	for _, resp := range attestationsResp {
		bundle := resp.Bundle
		attBytes, err := json.Marshal(bundle)
		if err != nil {
			if err = f.Close(); err != nil {
				return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to close file while marshalling JSON: %v", err))
			}
			return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to marshall attestation to JSON while writing to file: %v", err))
		}

		withNewline := fmt.Sprintf("%s\n", attBytes)
		_, err = f.Write([]byte(withNewline))
		if err != nil {
			if err = f.Close(); err != nil {
				return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to close file while handling write error: %v", err))
			}

			return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to write attestations: %v", err))
		}
	}

	if err = f.Close(); err != nil {
		return "", errors.Join(ErrAttestationFileCreation, fmt.Errorf("failed to close file after writing attestations: %v", err))
	}

	return metadataFilePath, nil
}

func NewLiveStore(outputPath string) *LiveStore {
	return &LiveStore{
		outputPath: outputPath,
	}
}
