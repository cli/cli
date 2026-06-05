package shared

import (
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/stretchr/testify/assert"
)

func TestFormatAssignees(t *testing.T) {
	tests := []struct {
		name     string
		issue    *api.Issue
		expected string
	}{
		{
			name: "no assignees",
			issue: &api.Issue{
				Assignees: api.Assignees{
					Nodes: []api.GitHubUser{},
				},
			},
			expected: "-",
		},
		{
			name: "one assignee",
			issue: &api.Issue{
				Assignees: api.Assignees{
					Nodes: []api.GitHubUser{
						{Login: "alice"},
					},
				},
			},
			expected: "alice",
		},
		{
			name: "two assignees",
			issue: &api.Issue{
				Assignees: api.Assignees{
					Nodes: []api.GitHubUser{
						{Login: "alice"},
						{Login: "bob"},
					},
				},
			},
			expected: "alice+1",
		},
		{
			name: "three assignees",
			issue: &api.Issue{
				Assignees: api.Assignees{
					Nodes: []api.GitHubUser{
						{Login: "alice"},
						{Login: "bob"},
						{Login: "charlie"},
					},
				},
			},
			expected: "alice+2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAssignees(tt.issue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Made with Bob
