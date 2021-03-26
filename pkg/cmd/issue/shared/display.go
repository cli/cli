package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/api"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
	"github.com/cli/cli/utils"
)

func PrintIssues(io *iostreams.IOStreams, prefix string, totalCount int, issues []api.Issue) {
	cs := io.ColorScheme()
	table := utils.NewTablePrinter(io)
	for _, issue := range issues {
		issueNum := strconv.Itoa(issue.Number)
		if table.IsTTY() {
			issueNum = "#" + issueNum
		}
		issueNum = prefix + issueNum
		labels := IssueLabelList(issue)
		if labels != "" && table.IsTTY() {
			labels = fmt.Sprintf("(%s)", labels)
		}
		now := time.Now()
		ago := now.Sub(issue.UpdatedAt)
		colorFunc := cs.ColorFromString(prShared.ColorForState(issue.State))
		issueURL := issue.URL
		table.AddField(issueNum, nil, func(t string) string {
			return colorFunc(cs.Hyperlink(t, issueURL))
		})
		if !table.IsTTY() {
			table.AddField(issue.State, nil, nil)
		}
		table.AddField(text.ReplaceExcessiveWhitespace(issue.Title), nil, nil)
		table.AddField(labels, truncateLabels, cs.Gray)
		if table.IsTTY() {
			table.AddField(utils.FuzzyAgo(ago), nil, cs.Gray)
		} else {
			table.AddField(issue.UpdatedAt.String(), nil, nil)
		}
		table.EndRow()
	}
	_ = table.Render()
	remaining := totalCount - len(issues)
	if remaining > 0 {
		fmt.Fprintf(io.Out, cs.Gray("%sAnd %d more\n"), prefix, remaining)
	}
}

func truncateLabels(w int, t string) string {
	if len(t) < 2 {
		return t
	}
	truncated := text.Truncate(w-2, t[1:len(t)-1])
	return fmt.Sprintf("(%s)", truncated)
}

func IssueLabelList(issue api.Issue) string {
	if len(issue.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(issue.Labels.Nodes))
	for _, label := range issue.Labels.Nodes {
		labelNames = append(labelNames, label.Name)
	}

	return strings.Join(labelNames, ", ")
}
