package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_String(t *testing.T) {
	tests := map[string]struct {
		rg    ReactionGroup
		emoji string
		count int
	}{
		"empty reaction group": {
			rg:    ReactionGroup{},
			emoji: "",
			count: 0,
		},
		"unknown reaction group": {
			rg: ReactionGroup{
				Content: "UNKNOWN",
				Users:   ReactionGroupUsers{TotalCount: 1},
			},
			emoji: "",
			count: 1,
		},
		"thumbs up reaction group": {
			rg: ReactionGroup{
				Content: "THUMBS_UP",
				Users:   ReactionGroupUsers{TotalCount: 2},
			},
			emoji: "\U0001f44d",
			count: 2,
		},
		"thumbs down reaction group": {
			rg: ReactionGroup{
				Content: "THUMBS_DOWN",
				Users:   ReactionGroupUsers{TotalCount: 3},
			},
			emoji: "\U0001f44e",
			count: 3,
		},
		"laugh reaction group": {
			rg: ReactionGroup{
				Content: "LAUGH",
				Users:   ReactionGroupUsers{TotalCount: 4},
			},
			emoji: "\U0001f604",
			count: 4,
		},
		"hooray reaction group": {
			rg: ReactionGroup{
				Content: "HOORAY",
				Users:   ReactionGroupUsers{TotalCount: 5},
			},
			emoji: "\U0001f389",
			count: 5,
		},
		"confused reaction group": {
			rg: ReactionGroup{
				Content: "CONFUSED",
				Users:   ReactionGroupUsers{TotalCount: 6},
			},
			emoji: "\U0001f615",
			count: 6,
		},
		"heart reaction group": {
			rg: ReactionGroup{
				Content: "HEART",
				Users:   ReactionGroupUsers{TotalCount: 7},
			},
			emoji: "\u2764\ufe0f",
			count: 7,
		},
		"rocket reaction group": {
			rg: ReactionGroup{
				Content: "ROCKET",
				Users:   ReactionGroupUsers{TotalCount: 8},
			},
			emoji: "\U0001f680",
			count: 8,
		},
		"eyes reaction group": {
			rg: ReactionGroup{
				Content: "EYES",
				Users:   ReactionGroupUsers{TotalCount: 9},
			},
			emoji: "\U0001f440",
			count: 9,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.emoji, tt.rg.Emoji())
			assert.Equal(t, tt.count, tt.rg.Count())
		})
	}
}
