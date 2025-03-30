package artifact

import (
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/digest"
)

func digestLocalFileArtifact(filename, digestAlg string) (*DigestedArtifact, error) {
	data, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to get open local artifact: %v", err)
	}
	defer data.Close()
	digest, err := digest.CalculateDigestWithAlgorithm(data, digestAlg)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate local artifact digest: %v", err)
	}
	return &DigestedArtifact{
		URL:       fmt.Sprintf("file://%s", filename),
		digest:    digest,
		digestAlg: digestAlg,
	}, nil
}
