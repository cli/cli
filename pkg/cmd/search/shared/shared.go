package shared

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
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
	Browser  browser.Browser
	Entity   EntityType
	Exporter cmdutil.Exporter
	IO       *iostreams.IOStreams
	Now      time.Time
	Query    search.Query
	Searcher search.Searcher
	WebMode  bool
}

func Searcher(f *cmdutil.Factory) (search.Searcher, error) {
	cfg, err := f.Config()
	if err != nil {
		return nil, err
	}
	host, _ := cfg.Authentication().DefaultHost()
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
			fmt.Fprintf(io.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
	}
	io.StartProgressIndicator()
	result, err := opts.Searcher.Issues(opts.Query)
	io.StopProgressIndicator()
	if err != nil {
		return err
	}
	if len(result.Items) == 0 && opts.Exporter == nil {
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

	if err := io.StartPager(); err == nil {
		defer io.StopPager()
	} else {
		fmt.Fprintf(io.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(io, result.Items)
	}

	return displayIssueResults(io, opts.Now, opts.Entity, result)
}

func displayIssueResults(io *iostreams.IOStreams, now time.Time, et EntityType, results search.IssuesResult) error {
	if now.IsZero() {
		now = time.Now()
	}

	var headers []string
	if et == Both {
		headers = []string{"Kind", "Repo", "ID", "Title", "Labels", "Updated"}
	} else {
		headers = []string{"Repo", "ID", "Title", "Labels", "Updated"}
	}

	cs := io.ColorScheme()
	tp := tableprinter.New(io, tableprinter.WithHeader(headers...))
	for _, issue := range results.Items {
		if et == Both {
			kind := "issue"
			if issue.IsPullRequest() {
				kind = "pr"
			}
			tp.AddField(kind)
		}
		comp := strings.Split(issue.RepositoryURL, "/")
		name := comp[len(comp)-2:]
		tp.AddField(strings.Join(name, "/"))
		issueNum := strconv.Itoa(issue.Number)
		if tp.IsTTY() {
			issueNum = "#" + issueNum
		}
		if issue.IsPullRequest() {
			color := tableprinter.WithColor(cs.ColorFromString(colorForPRState(issue.State())))
			tp.AddField(issueNum, color)
		} else {
			color := tableprinter.WithColor(cs.ColorFromString(colorForIssueState(issue.State(), issue.StateReason)))
			tp.AddField(issueNum, color)
		}
		if !tp.IsTTY() {
			tp.AddField(issue.State())
		}
		tp.AddField(text.RemoveExcessiveWhitespace(issue.Title))
		tp.AddField(listIssueLabels(&issue, cs, tp.IsTTY()))
		tp.AddTimeField(now, issue.UpdatedAt, cs.Gray)
		tp.EndRow()
	}

	if tp.IsTTY() {
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

func colorForIssueState(state, reason string) string {
	switch state {
	case "open":
		return "green"
	case "closed":
		if reason == "not_planned" {
			return "gray"
		}
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
