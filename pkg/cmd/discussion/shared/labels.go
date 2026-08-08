package shared

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
)

// ResolveLabels matches user-provided label names (case-insensitive) against a
// set of known labels and returns the corresponding IDs. If any names cannot be
// matched, all unrecognized names are reported in the returned error.
func ResolveLabels(allLabels []client.DiscussionLabel, names []string) ([]string, error) {
	byName := make(map[string]string, len(allLabels))
	for _, l := range allLabels {
		byName[strings.ToLower(l.Name)] = l.ID
	}

	var ids []string
	var missing []string

	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		id, ok := byName[strings.ToLower(trimmed)]
		if !ok {
			missing = append(missing, trimmed)
		} else {
			ids = append(ids, id)
		}
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("labels not found: %s", strings.Join(missing, ", "))
	}

	return ids, nil
}
