package label

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSortLabels(t *testing.T) {
	labels := []label{
		{
			Name:        "documentation",
			Color:       "0075ca",
			Description: "Improvements or additions to documentation",
			CreatedAt:   time.Date(2022, 04, 20, 6, 17, 50, 0, time.UTC),
		},
		{
			Name:        "bug",
			Color:       "d73a4a",
			Description: "Something isn't working",
			CreatedAt:   time.Date(2022, 04, 20, 6, 17, 50, 0, time.UTC),
		},
		{
			Name:        "duplicate",
			Color:       "cfd3d7",
			Description: "This issue or pull request already exists",
			CreatedAt:   time.Date(2022, 04, 20, 6, 17, 49, 0, time.UTC),
		},
	}

	tests := []struct {
		name      string
		ascending bool
		field     string
		expected  []string
	}{
		{
			name:      "name asc",
			ascending: true,
			field:     "name",
			expected:  []string{"bug", "documentation", "duplicate"},
		},
		{
			name:      "name desc",
			ascending: false,
			field:     "name",
			expected:  []string{"duplicate", "documentation", "bug"},
		},
		{
			name:      "created asc",
			ascending: true,
			field:     "created",
			expected:  []string{"duplicate", "bug", "documentation"},
		},
		{
			name:      "created desc",
			ascending: false,
			field:     "created",
			expected:  []string{"documentation", "bug", "duplicate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortLabels(labels, tt.field, tt.ascending)

			actual := make([]string, len(labels))
			for i := range labels {
				actual[i] = labels[i].Name
			}

			assert.EqualValues(t, tt.expected, actual)
		})
	}
}
