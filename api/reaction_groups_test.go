package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_String(t *testing.T) {
	tests := map[string]struct {
		rgs    ReactionGroups
		output string
	}{
		"empty reaction groups": {
			rgs:    []ReactionGroup{},
			output: `^$`,
		},
		"non-empty reaction groups": {
			rgs: []ReactionGroup{
				ReactionGroup{
					Content: "LAUGH",
					Users:   ReactionGroupUsers{TotalCount: 0},
				},
				ReactionGroup{
					Content: "HOORAY",
					Users:   ReactionGroupUsers{TotalCount: 1},
				},
				ReactionGroup{
					Content: "CONFUSED",
					Users:   ReactionGroupUsers{TotalCount: 0},
				},
				ReactionGroup{
					Content: "HEART",
					Users:   ReactionGroupUsers{TotalCount: 2},
				},
			},
			output: `^1 \x{1f389} • 2 \x{2764}\x{fe0f}$`,
		},
		"reaction groups with unmapped emoji": {
			rgs: []ReactionGroup{
				ReactionGroup{
					Content: "UNKNOWN",
					Users:   ReactionGroupUsers{TotalCount: 1},
				},
			},
			output: `^$`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Regexp(t, tt.output, tt.rgs.String())
		})
	}
}
