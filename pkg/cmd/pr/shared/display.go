package shared

import (
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/cli/cli/v2/utils"
)

func StateTitleWithColor(cs *iostreams.ColorScheme, pr api.PullRequest) string {
	prStateColorFunc := cs.ColorFromString(ColorForPRState(pr))
	if pr.State == "OPEN" && pr.IsDraft {
		return prStateColorFunc("Draft")
	}
	return prStateColorFunc(text.Title(pr.State))
}

func ColorForPRState(pr api.PullRequest) string {
	switch pr.State {
	case "OPEN":
		if pr.IsDraft {
			return "gray"
		}
		return "green"
	case "CLOSED":
		return "red"
	case "MERGED":
		return "magenta"
	default:
		return ""
	}
}

func ColorForIssueState(issue api.Issue) string {
	switch issue.State {
	case "OPEN":
		return "green"
	case "CLOSED":
		return "magenta"
	default:
		return ""
	}
}

func PrintHeader(io *iostreams.IOStreams, s string) {
	fmt.Fprintln(io.Out, io.ColorScheme().Bold(s))
}

func PrintMessage(io *iostreams.IOStreams, s string) {
	fmt.Fprintln(io.Out, io.ColorScheme().Gray(s))
}

func ListNoResults(repoName string, itemName string, hasFilters bool) error {
	if hasFilters {
		return cmdutil.NewNoResultsError(fmt.Sprintf("no %ss match your search in %s", itemName, repoName))
	}
	return cmdutil.NewNoResultsError(fmt.Sprintf("no open %ss in %s", itemName, repoName))
}

func ListHeader(repoName string, itemName string, matchCount int, totalMatchCount int, hasFilters bool) string {
	if hasFilters {
		matchVerb := "match"
		if totalMatchCount == 1 {
			matchVerb = "matches"
		}
		return fmt.Sprintf("Showing %d of %s in %s that %s your search", matchCount, utils.Pluralize(totalMatchCount, itemName), repoName, matchVerb)
	}

	return fmt.Sprintf("Showing %d of %s in %s", matchCount, utils.Pluralize(totalMatchCount, fmt.Sprintf("open %s", itemName)), repoName)
}
