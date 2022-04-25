package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/cli/cli/v2/utils"
)

type EntityType int

const (
	// Limitation of GitHub search see:
	// https://docs.github.com/en/rest/reference/search
	SearchMaxResults = 1000

	Both EntityType = iota
	Issues
	PullRequests
)

type IssuesOptions struct {
	Browser  cmdutil.Browser
	Entity   EntityType
	Exporter cmdutil.Exporter
	IO       *iostreams.IOStreams
	Query    search.Query
	Searcher search.Searcher
	WebMode  bool
}

func Searcher(f *cmdutil.Factory) (search.Searcher, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, err
	}
	host, err := cfg.DefaultHost()
	if err != nil {
		return nil, err
	}
	client, err := f.HttpClient()
	if err != nil {
		return nil, err
	}
	return search.NewSearcher(client, host), nil
}

func SearchIssues(opts *IssuesOptions) error {
	io := opts.IO
	if opts.WebMode {
		url := opts.Searcher.URL(opts.Query)
		if io.IsStdoutTTY() {
			fmt.Fprintf(io.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
	}
	io.StartProgressIndicator()
	result, err := opts.Searcher.Issues(opts.Query)
	io.StopProgressIndicator()
	if err != nil {
		return err
	}

	if err := io.StartPager(); err == nil {
		defer io.StopPager()
	} else {
		fmt.Fprintf(io.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(io, result.Items)
	}

	if len(result.Items) == 0 {
		var msg string
		switch opts.Entity {
		case Both:
			msg = "no issues or pull requests matched your search"
		case Issues:
			msg = "no issues matched your search"
		case PullRequests:
			msg = "no pull requests matched your search"
		}
		return cmdutil.NewNoResultsError(msg)
	}

	return displayIssueResults(io, opts.Entity, result)
}

func displayIssueResults(io *iostreams.IOStreams, et EntityType, results search.IssuesResult) error {
	cs := io.ColorScheme()
	tp := utils.NewTablePrinter(io)
	for _, issue := range results.Items {
		if et == Both {
			kind := "issue"
			if issue.IsPullRequest() {
				kind = "pr"
			}
			tp.AddField(kind, nil, nil)
		}
		comp := strings.Split(issue.RepositoryURL, "/")
		name := comp[len(comp)-2:]
		tp.AddField(strings.Join(name, "/"), nil, nil)
		issueNum := strconv.Itoa(issue.Number)
		if tp.IsTTY() {
			issueNum = "#" + issueNum
		}
		if issue.IsPullRequest() {
			tp.AddField(issueNum, nil, cs.ColorFromString(colorForPRState(issue.State)))
		} else {
			tp.AddField(issueNum, nil, cs.ColorFromString(colorForIssueState(issue.State)))
		}
		if !tp.IsTTY() {
			tp.AddField(issue.State, nil, nil)
		}
		tp.AddField(text.ReplaceExcessiveWhitespace(issue.Title), nil, nil)
		tp.AddField(listIssueLabels(&issue, cs, tp.IsTTY()), nil, nil)
		now := time.Now()
		ago := now.Sub(issue.UpdatedAt)
		if tp.IsTTY() {
			tp.AddField(utils.FuzzyAgo(ago), nil, cs.Gray)
		} else {
			tp.AddField(issue.UpdatedAt.String(), nil, nil)
		}
		tp.EndRow()
	}

	if io.IsStdoutTTY() {
		var header string
		switch et {
		case Both:
			header = fmt.Sprintf("Showing %d of %d issues and pull requests\n\n", len(results.Items), results.Total)
		case Issues:
			header = fmt.Sprintf("Showing %d of %d issues\n\n", len(results.Items), results.Total)
		case PullRequests:
			header = fmt.Sprintf("Showing %d of %d pull requests\n\n", len(results.Items), results.Total)
		}
		fmt.Fprintf(io.Out, "\n%s", header)
	}
	return tp.Render()
}

func listIssueLabels(issue *search.Issue, cs *iostreams.ColorScheme, colorize bool) string {
	if len(issue.Labels) == 0 {
		return ""
	}
	labelNames := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		if colorize {
			labelNames = append(labelNames, cs.HexToRGB(label.Color, label.Name))
		} else {
			labelNames = append(labelNames, label.Name)
		}
	}
	return strings.Join(labelNames, ", ")
}

func colorForIssueState(state string) string {
	switch state {
	case "open":
		return "green"
	case "closed":
		return "magenta"
	default:
		return ""
	}
}

func colorForPRState(state string) string {
	switch state {
	case "open":
		return "green"
	case "closed":
		return "red"
	case "merged":
		return "magenta"
	default:
		return ""
	}
}
