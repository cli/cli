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
		now := time.Now()
		ago := now.Sub(issue.UpdatedAt)
		table.AddField(issueNum, nil, cs.ColorFromString(prShared.ColorForState(issue.State)))
		if !table.IsTTY() {
			table.AddField(issue.State, nil, nil)
		}
		table.AddField(text.ReplaceExcessiveWhitespace(issue.Title), nil, nil)
		table.AddField(issueLabelList(&issue, cs, table.IsTTY()), nil, nil)
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

func issueLabelList(issue *api.Issue, cs *iostreams.ColorScheme, colorize bool) string {
	if len(issue.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(issue.Labels.Nodes))
	for _, label := range issue.Labels.Nodes {
		if colorize {
			labelNames = append(labelNames, cs.HexToRGB(label.Color, label.Name))
		} else {
			labelNames = append(labelNames, label.Name)
		}
	}

	return strings.Join(labelNames, ", ")
}
