package extensions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOfficialExtension_Repository(t *testing.T) {
	ext := &OfficialExtension{Name: "stack", Owner: "github", Repo: "gh-stack"}
	repo := ext.Repository()
	assert.Equal(t, "github", repo.RepoOwner())
	assert.Equal(t, "gh-stack", repo.RepoName())
	assert.Equal(t, "github.com", repo.RepoHost())
}

func TestIsOfficial(t *testing.T) {
	tests := []struct {
		name     string
		extName  string
		extOwner string
		want     bool
	}{
		{
			name:     "known official extension matches",
			extName:  "stack",
			extOwner: "github",
			want:     true,
		},
		{
			name:     "official name with different owner is not official",
			extName:  "stack",
			extOwner: "williammartin",
			want:     false,
		},
		{
			name:     "official name with empty owner is not official",
			extName:  "stack",
			extOwner: "",
			want:     false,
		},
		{
			name:     "owner comparison is case-insensitive",
			extName:  "stack",
			extOwner: "GitHub",
			want:     true,
		},
		{
			name:     "mixed-case name does not match",
			extName:  "STACK",
			extOwner: "github",
			want:     false,
		},
		{
			name:     "unknown name is not official",
			extName:  "not-a-real-extension",
			extOwner: "github",
			want:     false,
		},
		{
			name:     "empty name is not official",
			extName:  "",
			extOwner: "github",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsOfficial(tt.extName, tt.extOwner))
		})
	}
}
