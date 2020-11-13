package api

import (
	"fmt"
	"strings"
)

type ReactionGroups []ReactionGroup

type ReactionGroup struct {
	Content string
	Users   ReactionGroupUsers
}

type ReactionGroupUsers struct {
	TotalCount int
}

func (rg ReactionGroup) String() string {
	c := rg.Users.TotalCount
	if c == 0 {
		return ""
	}
	e := reactionEmoji[rg.Content]
	if e == "" {
		return ""
	}
	return fmt.Sprintf("%v %s", c, e)
}

func (rgs ReactionGroups) String() string {
	var rs []string

	for _, rg := range rgs {
		if r := rg.String(); r != "" {
			rs = append(rs, r)
		}
	}

	return strings.Join(rs, " • ")
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
