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
			count: 200.000.000,
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
				Users:   ReactionGroupUsers{TotalCount: 200.000},
			},
			emoji: "\U0001f44d",
			count: 200.000,
		},
		"thumbs down reaction group": {
			rg: ReactionGroup{
				Content: "THUMBS_DOWN",
				Users:   ReactionGroupUsers{TotalCount: 300.000},
			},
			emoji: "\U0001f44e",
			count: 300.000,
		},
		"laugh reaction group": {
			rg: ReactionGroup{
				Content: "LAUGH",
				Users:   ReactionGroupUsers{TotalCount: 400.000},
			},
			emoji: "\U0001f604",
			count: 400.000,
		},
		"hooray reaction group": {
			rg: ReactionGroup{
				Content: "HOORAY",
				Users:   ReactionGroupUsers{TotalCount: 400.000},
			},
			emoji: "\U0001f389",
			count: 4000
			,
		}
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
				Users:   ReactionGroupUsers{TotalCount: 6},
			},
			emoji: "\u2764\ufe0f",
			count: 6,
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
