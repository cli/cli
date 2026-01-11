//go:build !windows

package artifact

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/stretchr/testify/require"
)

func TestNormalizeReference(t *testing.T) {
	testCases := []struct {
		name           string
		reference      string
		pathSeparator  rune
		expectedResult string
		expectedType   artifactType
		expectedError  bool
	}{
		{
			name:           "file reference without scheme",
			reference:      "/path/to/file",
			pathSeparator:  '/',
			expectedResult: "/path/to/file",
			expectedType:   fileArtifactType,
			expectedError:  false,
		},
		{
			name:           "file scheme uri with %20",
			reference:      "file:///path/to/file%20with%20spaces",
			pathSeparator:  '/',
			expectedResult: "/path/to/file with spaces",
			expectedType:   fileArtifactType,
			expectedError:  false,
		},
		{
			name:           "windows file reference without scheme",
			reference:      `c:\path\to\file`,
			pathSeparator:  '\\',
			expectedResult: `c:\path\to\file`,
			expectedType:   fileArtifactType,
			expectedError:  false,
		},
		{
			name:           "file reference with scheme",
			reference:      "file:///path/to/file",
			pathSeparator:  '/',
			expectedResult: "/path/to/file",
			expectedType:   fileArtifactType,
			expectedError:  false,
		},
		{
			name:           "windows path",
			reference:      "file:///C:/path/to/file",
			pathSeparator:  '\\',
			expectedResult: `C:\path\to\file`,
			expectedType:   fileArtifactType,
			expectedError:  false,
		},
		{
			name:           "windows path with backslashes",
			reference:      "file:///C:\\path\\to\\file",
			pathSeparator:  '\\',
			expectedResult: `C:\path\to\file`,
			expectedType:   fileArtifactType,
			expectedError:  false,
		},
		{
			name:           "oci reference",
			reference:      "oci://example.com/repo:tag",
			pathSeparator:  '/',
			expectedResult: "example.com/repo:tag",
			expectedType:   ociArtifactType,
			expectedError:  false,
		},
		{
			name:           "oci reference with digest",
			reference:      "oci://example.com/repo@sha256:abcdef1234567890",
			pathSeparator:  '/',
			expectedResult: "example.com/repo@sha256:abcdef1234567890",
			expectedType:   ociArtifactType,
			expectedError:  false,
		},
		{
			name:           "sha256 digest reference",
			reference:      "sha256:abcdef1234567890",
			pathSeparator:  '/',
			expectedResult: "sha256:abcdef1234567890",
			expectedType:   digestArtifactType,
			expectedError:  false,
		},
		{
			name:           "sha512 digest reference",
			reference:      "sha512:abcdef1234567890abcdef",
			pathSeparator:  '/',
			expectedResult: "sha512:abcdef1234567890abcdef",
			expectedType:   digestArtifactType,
			expectedError:  false,
		},
		{
			name:           "file starting with sha256 but not a digest",
			reference:      "sha256_checksums.txt",
			pathSeparator:  '/',
			expectedResult: "sha256_checksums.txt",
			expectedType:   fileArtifactType,
			expectedError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, artifactType, err := normalizeReference(tc.reference, tc.pathSeparator)
			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedResult, result)
				require.Equal(t, tc.expectedType, artifactType)
			}
		})
	}
}

func TestParseDigestReference(t *testing.T) {
	testCases := []struct {
		name          string
		reference     string
		expectedAlg   string
		expectedValue string
		expectedError bool
	}{
		{
			name:          "valid sha256 digest",
			reference:     "sha256:abcdef1234567890",
			expectedAlg:   "sha256",
			expectedValue: "abcdef1234567890",
			expectedError: false,
		},
		{
			name:          "valid sha512 digest",
			reference:     "sha512:abcdef1234567890abcdef",
			expectedAlg:   "sha512",
			expectedValue: "abcdef1234567890abcdef",
			expectedError: false,
		},
		{
			name:          "unsupported algorithm",
			reference:     "sha384:abcdef1234567890",
			expectedError: true,
		},
		{
			name:          "uppercase algorithm normalized",
			reference:     "SHA256:abcdef1234567890",
			expectedAlg:   "sha256",
			expectedValue: "abcdef1234567890",
			expectedError: false,
		},
		{
			name:          "invalid format no colon",
			reference:     "sha256abcdef",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			alg, value, err := parseDigestReference(tc.reference)
			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedAlg, alg)
				require.Equal(t, tc.expectedValue, value)
			}
		})
	}
}

func TestNewDigestedArtifactWithDigestInput(t *testing.T) {
	mockClient := oci.MockClient{}

	t.Run("sha256 digest input", func(t *testing.T) {
		artifact, err := NewDigestedArtifact(&mockClient, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "sha256")
		require.NoError(t, err)
		require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", artifact.Digest())
		require.Equal(t, "sha256", artifact.Algorithm())
		require.Equal(t, "", artifact.URL)
	})

	t.Run("sha512 digest input", func(t *testing.T) {
		artifact, err := NewDigestedArtifact(&mockClient, "sha512:cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e", "sha256") // The argument `digestAlg` is ignored if `reference` is a digest reference.
		require.NoError(t, err)
		require.Equal(t, "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e", artifact.Digest())
		require.Equal(t, "sha512", artifact.Algorithm())
		require.Equal(t, "", artifact.URL)
	})

	t.Run("unsupported algorithm treated as file path", func(t *testing.T) {
		// sha384: is not a recognized prefix, so it's treated as a file path
		_, err := NewDigestedArtifact(&mockClient, "sha384:abcdef", "sha256")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to open local artifact")
	})
}
