package api

import (
	"bytes"
	"encoding/json"
)

type ReactionGroups []ReactionGroup

func (rg ReactionGroups) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteRune('[')
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	hasPrev := false
	for _, g := range rg {
		if g.Users.TotalCount == 0 {
			continue
		}
		if hasPrev {
			buf.WriteRune(',')
		}
		if err := encoder.Encode(&g); err != nil {
			return nil, err
		}
		hasPrev = true
	}
	buf.WriteRune(']')
	return buf.Bytes(), nil
}

type ReactionGroup struct {
	Content string             `json:"content"`
	Users   ReactionGroupUsers `json:"users"`
}

type ReactionGroupUsers struct {
	TotalCount int `json:"totalCount"`
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
