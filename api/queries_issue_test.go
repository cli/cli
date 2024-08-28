package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Labels_SortAlphabeticallyIgnoreCase(t *testing.T) {
	tests := map[string]struct {
		labels   Labels
		expected Labels
	}{
		"no repeat labels": {
			labels: Labels{
				Nodes: []IssueLabel{
					{Name: "c"},
					{Name: "B"},
					{Name: "a"},
				},
			},
			expected: Labels{
				Nodes: []IssueLabel{
					{Name: "a"},
					{Name: "B"},
					{Name: "c"},
				},
			},
		},
		"repeat labels case insensitive": {
			labels: Labels{
				Nodes: []IssueLabel{
					{Name: "c"},
					{Name: "B"},
					{Name: "C"},
				},
			},
			expected: Labels{
				Nodes: []IssueLabel{
					{Name: "B"},
					{Name: "c"},
					{Name: "C"},
				},
			},
		},
		"no labels": {
			labels: Labels{
				Nodes: []IssueLabel{},
			},
			expected: Labels{
				Nodes: []IssueLabel{},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.labels.SortAlphabeticallyIgnoreCase()
			assert.Equal(t, tc.expected, tc.labels)
		})
	}
}
