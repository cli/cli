package shared

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
)

// MatchCategory finds a category by name or slug (case-insensitive).
// It prefers an exact slug match over a name match, so users are
// encouraged to use slugs for unambiguous lookups.
func MatchCategory(input string, categories []client.DiscussionCategory) (*client.DiscussionCategory, error) {
	for i := range categories {
		if strings.EqualFold(categories[i].Slug, input) {
			return &categories[i], nil
		}
	}
	for i := range categories {
		if strings.EqualFold(categories[i].Name, input) {
			return &categories[i], nil
		}
	}

	slugs := make([]string, len(categories))
	for i, c := range categories {
		slugs[i] = c.Slug
	}
	slices.Sort(slugs)
	return nil, fmt.Errorf("unknown category: %q; must be one of: %s", input, strings.Join(slugs, ", "))
}
