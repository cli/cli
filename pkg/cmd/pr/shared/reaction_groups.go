package shared

import (
	"fmt"
	"strings"

	"github.com/cli/cli/api"
)

func ReactionGroupList(rgs api.ReactionGroups) string {
	var rs []string

	for _, rg := range rgs {
		if r := formatReactionGroup(rg); r != "" {
			rs = append(rs, r)
		}
	}

	return strings.Join(rs, " • ")
}

func formatReactionGroup(rg api.ReactionGroup) string {
	c := rg.Count()
	if c == 0 {
		return ""
	}
	e := rg.Emoji()
	if e == "" {
		return ""
	}
	return fmt.Sprintf("%v %s", c, e)
}
