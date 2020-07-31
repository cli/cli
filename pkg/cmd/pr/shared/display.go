package shared

import (
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/utils"
)

func StateTitleWithColor(pr api.PullRequest) string {
	prStateColorFunc := ColorFuncForPR(pr)
	if pr.State == "OPEN" && pr.IsDraft {
		return prStateColorFunc(strings.Title(strings.ToLower("Draft")))
	}
	return prStateColorFunc(strings.Title(strings.ToLower(pr.State)))
}

func ColorFuncForPR(pr api.PullRequest) func(string) string {
	if pr.State == "OPEN" && pr.IsDraft {
		return utils.Gray
	}
	return ColorFuncForState(pr.State)
}

// ColorFuncForState returns a color function for a PR/Issue state
func ColorFuncForState(state string) func(string) string {
	switch state {
	case "OPEN":
		return utils.Green
	case "CLOSED":
		return utils.Red
	case "MERGED":
		return utils.Magenta
	default:
		return nil
	}
}
