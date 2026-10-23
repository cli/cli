package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
