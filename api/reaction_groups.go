package api

type ReactionGroups []ReactionGroup

type ReactionGroup struct {
	Content string
	Users   ReactionGroupUsers
}

type ReactionGroupUsers struct {
	TotalCount int
}

func (rg ReactionGroup) Count() int {
	return rg.Users.TotalCount
}

func (rg ReactionGroup) Emoji() string {
	return reactionEmoji[rg.Content]
}

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

func reactionGroupsFragment() string {
	return `reactionGroups {
						content
						users {
							totalCount
						}
					}`
}
