package shared

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
)

var reactionEmoji = map[string]string{
	"THUMBS_UP":   "\U0001f44d",
	"THUMBS_DOWN": "\U0001f44e",
	"LAUGH":       "\U0001f604",
	"HOORAY":      "\U0001f389",
	"CONFUSED":    "\U0001f615",
	"HEART":       "\u2764\ufe0f",
	"ROCKET":      "\U0001f680",
	"EYES":        "\U0001f440",
}

// ReactionGroupList formats reaction groups for display.
func ReactionGroupList(groups []client.ReactionGroup) string {
	var parts []string
	for _, g := range groups {
		if g.TotalCount == 0 {
			continue
		}
		emoji := reactionEmoji[g.Content]
		if emoji == "" {
			emoji = g.Content
		}
		parts = append(parts, fmt.Sprintf("%s %d", emoji, g.TotalCount))
	}
	return strings.Join(parts, " • ")
}
