package artifact

import (
	"fmt"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/stretchr/testify/assert"
)

func TestDigestContainerImageArtifact(t *testing.T) {
	expectedDigest := "1234567890abcdef"
	client := oci.NewMockClient()
	url := "example.com/repo:tag"
	digestedArtifact, err := digestContainerImageArtifact(url, client)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("oci://%s", url), digestedArtifact.URL)
	assert.Equal(t, expectedDigest, digestedArtifact.digest)
	assert.Equal(t, "sha256", digestedArtifact.digestAlg)
}

func TestParseImageRefFailure(t *testing.T) {
	client := oci.NewReferenceFailClient()
	url := "example.com/repo:tag"
	_, err := digestContainerImageArtifact(url, client)
	assert.Error(t, err)
}

func TestFetchImageFailure(t *testing.T) {
	testcase := []struct {
		name        string
		client      *oci.Client
		expectedErr error
	}{
		{
			name:        "Fail to authorize with registry",
			client:      oci.NewAuthFailClient(),
			expectedErr: oci.ErrRegistryAuthz,
		},
		{
			name:        "Fail to fetch image due to denial",
			client:      oci.NewDeniedClient(),
			expectedErr: oci.ErrDenied,
		},
	}

	for _, tc := range testcase {
		url := "example.com/repo:tag"
		_, err := digestContainerImageArtifact(url, tc.client)
		assert.ErrorIs(t, err, tc.expectedErr)
	}
}
