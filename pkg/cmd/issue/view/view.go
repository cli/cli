package view

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	issueShared "github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser
	Detector   fd.Detector

	SelectorArg string
	WebMode     bool
	Comments    bool
	Exporter    cmdutil.Exporter

	Now func() time.Time

	IssuePrinter IssuePrinter
}

type IssuePrinter interface {
	Print(*PresentationIssue, ghrepo.Interface) error
}

type RawIssuePrinter struct {
	IO       *iostreams.IOStreams
	TimeNow  time.Time
	Comments bool
}

func (p *RawIssuePrinter) Print(issue *PresentationIssue, repo ghrepo.Interface) error {
	if p.Comments {
		fmt.Fprint(p.IO.Out, prShared.RawCommentList(issue.Comments, api.PullRequestReviews{}))
		return nil
	}

	ipf := NewIssuePrintFormatter(issue, p.IO, p.TimeNow, repo)
	return ipf.renderRawIssuePreview()
}

type RichIssuePrinter struct {
	IO       *iostreams.IOStreams
	TimeNow  time.Time
	Comments bool
}

func (p *RichIssuePrinter) Print(issue *PresentationIssue, repo ghrepo.Interface) error {
	ipf := NewIssuePrintFormatter(issue, p.IO, p.TimeNow, repo)
	isCommentsPreview := !p.Comments
	return ipf.renderHumanIssuePreview(isCommentsPreview)
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Now:        time.Now,
	}

	cmd := &cobra.Command{
		Use:   "view {<number> | <url>}",
		Short: "View an issue",
		Long: heredoc.Docf(`
			Display the title, body, and other information about an issue.

			With %[1]s--web%[1]s flag, open the issue in a web browser instead.
		`, "`"),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if opts.IO.IsStdoutTTY() {
				opts.IssuePrinter = &RichIssuePrinter{
					IO:       opts.IO,
					TimeNow:  opts.Now(),
					Comments: opts.Comments,
				}
			} else {
				opts.IssuePrinter = &RawIssuePrinter{
					IO:       opts.IO,
					TimeNow:  opts.Now(),
					Comments: opts.Comments,
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open an issue in the browser")
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "View issue comments")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.IssueFields)

	return cmd
}

var defaultFields = []string{
	"number", "url", "state", "createdAt", "title", "body", "author", "milestone",
	"assignees", "labels", "projectCards", "reactionGroups", "lastComment", "stateReason",
}

func viewRun(opts *ViewOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	lookupFields := set.NewStringSet()
	if opts.Exporter != nil {
		lookupFields.AddValues(opts.Exporter.Fields())
	} else if opts.WebMode {
		lookupFields.Add("url")
	} else {
		lookupFields.AddValues(defaultFields)
		if opts.Comments {
			lookupFields.Add("comments")
			lookupFields.Remove("lastComment")
		}
	}

	opts.IO.DetectTerminalTheme()

	opts.IO.StartProgressIndicator()
	issue, baseRepo, err := findIssue(httpClient, opts.BaseRepo, opts.SelectorArg, lookupFields.ToSlice(), opts.Detector)
	opts.IO.StopProgressIndicator()
	if err != nil {
		var loadErr *issueShared.PartialLoadError
		if opts.Exporter == nil && errors.As(err, &loadErr) {
			fmt.Fprintf(opts.IO.ErrOut, "warning: %s\n", loadErr.Error())
		} else {
			return err
		}
	}

	if opts.WebMode {
		openURL := issue.URL
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openURL))
		}
		return opts.Browser.Browse(openURL)
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, issue)
	}

	issue.Labels.SortAlphabeticallyIgnoreCase()

	presentationIssue, err := apiIssueToPresentationIssue(issue, opts.IO.ColorScheme())
	if err != nil {
		return err
	}

	return opts.IssuePrinter.Print(presentationIssue, baseRepo)
}

func findIssue(client *http.Client, baseRepoFn func() (ghrepo.Interface, error), selector string, fields []string, detector fd.Detector) (*api.Issue, ghrepo.Interface, error) {
	fieldSet := set.NewStringSet()
	fieldSet.AddValues(fields)
	fieldSet.Add("id")

	issue, repo, err := issueShared.IssueFromArgWithFields(client, baseRepoFn, selector, fieldSet.ToSlice(), detector)
	if err != nil {
		return issue, repo, err
	}

	if fieldSet.Contains("comments") {
		// FIXME: this re-fetches the comments connection even though the initial set of 100 were
		// fetched in the previous request.
		err = preloadIssueComments(client, repo, issue)
	}
	return issue, repo, err
}
