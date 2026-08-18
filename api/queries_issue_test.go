package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueRepositoryUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		json string
		want *PRRepository
	}{
		{
			name: "every field the repository selection asks for",
			json: `{"repository":{"id":"R_kgDOAAA","name":"REPO","nameWithOwner":"OWNER/REPO","databaseId":1234,"viewerPermission":"WRITE"}}`,
			want: &PRRepository{ID: "R_kgDOAAA", Name: "REPO", NameWithOwner: "OWNER/REPO", DatabaseID: 1234, ViewerPermission: "WRITE"},
		},
		{
			name: "nil when the field was never requested",
			json: `{"number":42}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Issue
			require.NoError(t, json.Unmarshal([]byte(tt.json), &got))

			assert.Equal(t, tt.want, got.Repository)
		})
	}
}

func TestIssueRepositoryDatabaseID(t *testing.T) {
	tests := []struct {
		name  string
		issue Issue
		want  int64
	}{
		{
			name:  "the repository field was requested",
			issue: Issue{Repository: &PRRepository{DatabaseID: 1234}},
			want:  1234,
		},
		{
			name:  "the repository field was not requested",
			issue: Issue{},
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.issue.RepositoryDatabaseID())
		})
	}
}

func TestIssueRepositoryViewerPermission(t *testing.T) {
	tests := []struct {
		name  string
		issue Issue
		want  string
	}{
		{
			name:  "the repository field was requested",
			issue: Issue{Repository: &PRRepository{ViewerPermission: "WRITE"}},
			want:  "WRITE",
		},
		{
			name:  "the repository field was not requested",
			issue: Issue{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.issue.RepositoryViewerPermission())
		})
	}
}
