package main

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func Test_findActiveInputMethodFlag(t *testing.T) {
	tests := []struct {
		name      string
		setInputs map[string]bool
		expected  string
		wantErr   bool
	}{
		{
			name: "No active flags",
			setInputs: map[string]bool{
				"-private-key": false,
				"-spki-file":   false,
				"-cert-file":   false,
			},
			expected: "",
			wantErr:  true,
		},
		{
			name: "Multiple active flags",
			setInputs: map[string]bool{
				"-private-key": true,
				"-spki-file":   true,
				"-cert-file":   false,
			},
			expected: "",
			wantErr:  true,
		},
		{
			name: "Single active flag",
			setInputs: map[string]bool{
				"-private-key": true,
				"-spki-file":   false,
				"-cert-file":   false,
			},
			expected: "-private-key",
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := findActiveInputMethodFlag(tc.setInputs)
			if tc.wantErr {
				test.AssertError(t, err, "findActiveInputMethodFlag() should have errored")
			} else {
				test.AssertNotError(t, err, "findActiveInputMethodFlag() should not have errored")
				test.AssertEquals(t, result, tc.expected)
			}
		})
	}
}
