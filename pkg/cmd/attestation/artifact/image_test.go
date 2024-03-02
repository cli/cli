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

func TestFetchImageFailure(t *testing.T) {
	client := oci.NewReferenceFailClient()
	url := "example.com/repo:tag"
	_, err := digestContainerImageArtifact(url, client)
	assert.Error(t, err)
}

func TestRegistryAuthFailure(t *testing.T) {
	client := oci.NewAuthFailClient()
	url := "example.com/repo:tag"
	_, err := digestContainerImageArtifact(url, client)
	assert.ErrorIs(t, err, oci.ErrRegistryAuthz)
}

func TestDeniedFailure(t *testing.T) {
	client := oci.NewDeniedClient()
	url := "example.com/repo:tag"
	_, err := digestContainerImageArtifact(url, client)
	assert.ErrorIs(t, err, oci.ErrDenied)
}
