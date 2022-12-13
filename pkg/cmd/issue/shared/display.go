package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/text"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
)

func PrintIssues(io *iostreams.IOStreams, now time.Time, prefix string, totalCount int, issues []api.Issue) {
	cs := io.ColorScheme()
	//nolint:staticcheck // SA1019: utils.NewTablePrinter is deprecated: use internal/tableprinter
	table := utils.NewTablePrinter(io)
	for _, issue := range issues {
		issueNum := strconv.Itoa(issue.Number)
		if table.IsTTY() {
			issueNum = "#" + issueNum
		}
		issueNum = prefix + issueNum
		table.AddField(issueNum, nil, cs.ColorFromString(prShared.ColorForIssueState(issue)))
		if !table.IsTTY() {
			table.AddField(issue.State, nil, nil)
		}
		table.AddField(text.RemoveExcessiveWhitespace(issue.Title), nil, nil)
		table.AddField(issueLabelList(&issue, cs, table.IsTTY()), nil, nil)
		if table.IsTTY() {
			table.AddField(text.FuzzyAgo(now, issue.UpdatedAt), nil, cs.Gray)
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
