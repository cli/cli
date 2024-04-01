//go:build !windows
// +build !windows

package artifact

import (
	"testing"

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
