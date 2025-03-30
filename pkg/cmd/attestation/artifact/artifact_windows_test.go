//go:build windows
// +build windows

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
			name:           "windows file reference without scheme",
			reference:      `c:\path\to\file`,
			pathSeparator:  '\\',
			expectedResult: `c:\path\to\file`,
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
