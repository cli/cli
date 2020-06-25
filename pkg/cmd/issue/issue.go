package issue

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type SharedOptions struct {
	IO           *iostreams.IOStreams
	HttpClient   func() (*http.Client, error)
	BaseRepo     func() (ghrepo.Interface, error)
	ColorableOut func() io.Writer
	ColorableErr func() io.Writer
}

func NewCmd(f *cmdutil.Factory, legacyIssueCmd *cobra.Command) *cobra.Command {
	sharedOpts := SharedOptions{
		IO:           f.IOStreams,
		HttpClient:   f.HttpClient,
		BaseRepo:     f.BaseRepo,
		ColorableOut: f.ColorableOut,
		ColorableErr: f.ColorableErr,
	}

	listCmd := newListCmd(sharedOpts, nil)
	legacyIssueCmd.AddCommand(listCmd)

	return legacyIssueCmd
}

func listHeader(repoName string, itemName string, matchCount int, totalMatchCount int, hasFilters bool) string {
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

	return fmt.Sprintf("Showing %d of %s in %s", matchCount, utils.Pluralize(totalMatchCount, itemName), repoName)
}

func printIssues(w io.Writer, prefix string, totalCount int, issues []api.Issue) {
	table := utils.NewTablePrinter(w)
	for _, issue := range issues {
		issueNum := strconv.Itoa(issue.Number)
		if table.IsTTY() {
			issueNum = "#" + issueNum
		}
		issueNum = prefix + issueNum
		labels := issueLabelList(issue)
		if labels != "" && table.IsTTY() {
			labels = fmt.Sprintf("(%s)", labels)
		}
		now := time.Now()
		ago := now.Sub(issue.UpdatedAt)
		table.AddField(issueNum, nil, colorFuncForState(issue.State))
		table.AddField(replaceExcessiveWhitespace(issue.Title), nil, nil)
		table.AddField(labels, nil, utils.Gray)
		table.AddField(utils.FuzzyAgo(ago), nil, utils.Gray)
		table.EndRow()
	}
	_ = table.Render()
	remaining := totalCount - len(issues)
	if remaining > 0 {
		fmt.Fprintf(w, utils.Gray("%sAnd %d more\n"), prefix, remaining)
	}
}

func issueLabelList(issue api.Issue) string {
	if len(issue.Labels.Nodes) == 0 {
		return ""
	}

	labelNames := make([]string, 0, len(issue.Labels.Nodes))
	for _, label := range issue.Labels.Nodes {
		labelNames = append(labelNames, label.Name)
	}

	list := strings.Join(labelNames, ", ")
	if issue.Labels.TotalCount > len(issue.Labels.Nodes) {
		list += ", …"
	}
	return list
}

// all funcs 👇 should be in a util package instead of issue.go because some PR commands use them too

// colorFuncForState returns a color function for a PR/Issue state
func colorFuncForState(state string) func(string) string {
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

func replaceExcessiveWhitespace(s string) string {
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`\r?\n`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\s{2,}`).ReplaceAllString(s, " ")
	return s
}
