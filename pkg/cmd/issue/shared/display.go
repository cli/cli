package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func PrintIssues(io *iostreams.IOStreams, now time.Time, prefix string, totalCount int, issues []api.Issue, showAssignees bool) {
	cs := io.ColorScheme()
	isTTY := io.IsStdoutTTY()
	headers := []string{"ID"}
	if !isTTY {
		headers = append(headers, "STATE")
	}
	headers = append(headers,
		"TITLE",
		"LABELS",
		"UPDATED",
	)
	if showAssignees {
		headers = append(headers, "ASSIGNEES")
	}
	table := tableprinter.New(io, tableprinter.WithHeader(headers...))
	for _, issue := range issues {
		issueNum := strconv.Itoa(issue.Number)
		if isTTY {
			issueNum = "#" + issueNum
		}
		issueNum = prefix + issueNum
		table.AddField(issueNum, tableprinter.WithColor(cs.ColorFromString(prShared.ColorForIssueState(issue))))
		if !isTTY {
			table.AddField(issue.State)
		}
		table.AddField(text.RemoveExcessiveWhitespace(issue.Title))
		table.AddField(issueLabelList(&issue, cs, isTTY))
		table.AddTimeField(now, issue.UpdatedAt, cs.Muted)
		if showAssignees {
			table.AddField(formatAssignees(&issue))
		}
		table.EndRow()
	}
	_ = table.Render()
	remaining := totalCount - len(issues)
	if remaining > 0 {
		fmt.Fprintf(io.Out, cs.Muted("%sAnd %d more\n"), prefix, remaining)
	}
}

func formatAssignees(issue *api.Issue) string {
	count := len(issue.Assignees.Nodes)
	if count == 0 {
		return "-"
	}
	if count == 1 {
		return issue.Assignees.Nodes[0].Login
	}
	return fmt.Sprintf("%s+%d", issue.Assignees.Nodes[0].Login, count-1)
}

func issueLabelList(issue *api.Issue, cs *iostreams.ColorScheme, colorize bool) string {
	if len(issue.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(issue.Labels.Nodes))
	for _, label := range issue.Labels.Nodes {
		if colorize {
			labelNames = append(labelNames, cs.Label(label.Color, label.Name))
		} else {
			labelNames = append(labelNames, label.Name)
		}
	}

	return strings.Join(labelNames, ", ")
}
