package download

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"

	"github.com/stretchr/testify/require"
)

func TestCreateJSONLinesFilePath(t *testing.T) {
	tempDir := t.TempDir()
	artifact, err := artifact.NewDigestedArtifact(oci.MockClient{}, "../test/data/sigstore-js-2.1.0.tgz", "sha512")
	require.NoError(t, err)

	outputFileName := fmt.Sprintf("%s.jsonl", artifact.DigestWithAlg())

	testCases := []struct {
		name       string
		outputPath string
		expected   string
	}{
		{
			name:       "with output path",
			outputPath: tempDir,
			expected:   path.Join(tempDir, outputFileName),
		},
		{
			name:       "with nested output path",
			outputPath: path.Join(tempDir, "subdir"),
			expected:   path.Join(tempDir, "subdir", outputFileName),
		},
		{
			name:       "with output path with beginning slash",
			outputPath: path.Join("/", tempDir, "subdir"),
			expected:   path.Join("/", tempDir, "subdir", outputFileName),
		},
		{
			name:       "without output path",
			outputPath: "",
			expected:   outputFileName,
		},
	}

	for _, tc := range testCases {
		store := LiveStore{
			tc.outputPath,
		}

		actualPath := store.createJSONLinesFilePath(artifact.DigestWithAlg())
		require.Equal(t, tc.expected, actualPath)
	}
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	counter := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		counter += 1
	}

	return counter, nil
}
