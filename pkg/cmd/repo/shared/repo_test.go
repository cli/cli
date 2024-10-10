package shared

import (
	"testing"
)

func TestNormalizeRepoName(t *testing.T) {
	// confirmed using GitHub.com/new
	tests := []struct {
		LocalName      string
		NormalizedName string
	}{
		{
			LocalName:      "cli",
			NormalizedName: "cli",
		},
		{
			LocalName:      "cli.git",
			NormalizedName: "cli",
		},
		{
			LocalName:      "@-#$^",
			NormalizedName: "---",
		},
		{
			LocalName:      "[cli]",
			NormalizedName: "-cli-",
		},
		{
			LocalName:      "Hello World, I'm a new repo!",
			NormalizedName: "Hello-World-I-m-a-new-repo-",
		},
		{
			LocalName:      " @E3H*(#$#_$-ZVp,n.7lGq*_eMa-(-zAZSJYg!",
			NormalizedName: "-E3H-_--ZVp-n.7lGq-_eMa---zAZSJYg-",
		},
		{
			LocalName:      "I'm a crazy .git repo name .git.git .git",
			NormalizedName: "I-m-a-crazy-.git-repo-name-.git.git-",
		},
	}
	for _, tt := range tests {
		output := NormalizeRepoName(tt.LocalName)
		if output != tt.NormalizedName {
			t.Errorf("Expected %q, got %q", tt.NormalizedName, output)
		}
	}
}
