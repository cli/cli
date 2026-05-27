package shared

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveLabels(t *testing.T) {
	tests := []struct {
		name      string
		allLabels []client.DiscussionLabel
		names     []string
		wantIDs   []string
		wantErr   string
	}{
		{
			name:      "empty source labels and empty names",
			allLabels: nil,
			names:     nil,
			wantIDs:   nil,
		},
		{
			name:      "empty source labels with non-empty names",
			allLabels: nil,
			names:     []string{"bug", "enhancement"},
			wantErr:   "labels not found: bug, enhancement",
		},
		{
			name: "non-empty source labels with empty names",
			allLabels: []client.DiscussionLabel{
				{ID: "L1", Name: "bug"},
				{ID: "L2", Name: "enhancement"},
			},
			names:   nil,
			wantIDs: nil,
		},
		{
			name: "all names match",
			allLabels: []client.DiscussionLabel{
				{ID: "L1", Name: "bug"},
				{ID: "L2", Name: "Enhancement"},
				{ID: "L3", Name: "documentation"},
			},
			names:   []string{"enhancement", "Bug"},
			wantIDs: []string{"L2", "L1"},
		},
		{
			name: "some names missing",
			allLabels: []client.DiscussionLabel{
				{ID: "L1", Name: "bug"},
				{ID: "L2", Name: "enhancement"},
			},
			names:   []string{"bug", "invalid", "unknown"},
			wantErr: "labels not found: invalid, unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := ResolveLabels(tt.allLabels, tt.names)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantIDs, ids)
			}
		})
	}
}
