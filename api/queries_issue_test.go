package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectCards_ProjectNames_NilNode(t *testing.T) {
	tests := []struct {
		name     string
		cards    ProjectCards
		expected []string
	}{
		{
			name: "all nil nodes",
			cards: ProjectCards{
				Nodes:      []*ProjectInfo{nil, nil},
				TotalCount: 2,
			},
			expected: nil,
		},
		{
			name: "mixed nil and valid nodes",
			cards: ProjectCards{
				Nodes: []*ProjectInfo{
					nil,
					{
						Project: struct {
							Name string `json:"name"`
						}{Name: "My Project"},
						Column: struct {
							Name string `json:"name"`
						}{Name: "In Progress"},
					},
					nil,
				},
				TotalCount: 3,
			},
			expected: []string{"My Project"},
		},
		{
			name: "no nodes",
			cards: ProjectCards{
				Nodes:      []*ProjectInfo{},
				TotalCount: 0,
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cards.ProjectNames()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProjectItems_ProjectTitles_NilNode(t *testing.T) {
	tests := []struct {
		name     string
		items    ProjectItems
		expected []string
	}{
		{
			name: "all nil nodes",
			items: ProjectItems{
				Nodes:      []*ProjectV2Item{nil, nil},
				TotalCount: 2,
			},
			expected: nil,
		},
		{
			name: "mixed nil and valid nodes",
			items: ProjectItems{
				Nodes: []*ProjectV2Item{
					nil,
					{
						ID: "ITEM1",
						Project: ProjectV2ItemProject{
							ID:    "PROJ1",
							Title: "v2 Project",
						},
						Status: ProjectV2ItemStatus{
							Name: "Done",
						},
					},
					nil,
				},
				TotalCount: 3,
			},
			expected: []string{"v2 Project"},
		},
		{
			name: "no nodes",
			items: ProjectItems{
				Nodes:      []*ProjectV2Item{},
				TotalCount: 0,
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.items.ProjectTitles()
			assert.Equal(t, tt.expected, result)
		})
	}
}
