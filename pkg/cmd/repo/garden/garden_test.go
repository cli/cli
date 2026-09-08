package garden

import (
	"testing"
)

func Test_computeSeed(t *testing.T) {
	tests := []struct {
		name     string
		repoName string
	}{
		{
			name:     "normal repo name",
			repoName: "cli/cli",
		},
		{
			name:     "short repo name",
			repoName: "a/b",
		},
		{
			name:     "empty repo name",
			repoName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seed := computeSeed(tt.repoName)
			if seed == 0 && tt.repoName != "" {
				t.Errorf("computeSeed() return 0 in unexpected way")
			}
		})
	}
}
