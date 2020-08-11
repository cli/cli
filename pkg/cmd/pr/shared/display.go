package shared

import (
	"fmt"
	"io"
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

func PrintHeader(w io.Writer, s string) {
	fmt.Fprintln(w, utils.Bold(s))
}

func PrintMessage(w io.Writer, s string) {
	fmt.Fprintln(w, utils.Gray(s))
}

func ListHeader(repoName string, itemName string, matchCount int, totalMatchCount int, hasFilters bool) string {
	if totalMatchCount == 0 {
		if hasFilters {
			return fmt.Sprintf("No %ss match your search in %s", itemName, repoName)
		}
		return fmt.Sprintf("There are no open %ss in %s", itemName, repoName)
	}

	if hasFilters {
		matchVerb := "match"
		if totalMatchCount == 1 {
			matchVerb = "matches"
		}
		return fmt.Sprintf("Showing %d of %s in %s that %s your search", matchCount, utils.Pluralize(totalMatchCount, itemName), repoName, matchVerb)
	}

	return fmt.Sprintf("Showing %d of %s in %s", matchCount, utils.Pluralize(totalMatchCount, fmt.Sprintf("open %s", itemName)), repoName)
}
