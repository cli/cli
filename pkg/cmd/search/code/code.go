package code

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/search/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

type CodeOptions struct {
	Browser  browser.Browser
	Exporter cmdutil.Exporter
	IO       *iostreams.IOStreams
	Query    search.Query
	Searcher search.Searcher
	WebMode  bool
}

func NewCmdCode(f *cmdutil.Factory, runF func(*CodeOptions) error) *cobra.Command {
	opts := &CodeOptions{
		Browser: f.Browser,
		IO:      f.IOStreams,
		Query:   search.Query{Kind: search.KindCode, TextMatch: true},
	}

	cmd := &cobra.Command{
		Use:   "code [<query>]",
		Short: "Search for code",
		Long: heredoc.Doc(`
			Search for code on GitHub.

			The command supports constructing queries using the GitHub search syntax,
			using the parameter and qualifier flags, or a combination of the two.

			At least one search term is required when searching code.

			GitHub search syntax is documented at:
			<https://docs.github.com/search-github/searching-on-github/searching-code>
		`),
		Example: heredoc.Doc(`
			# search code matching "react" and "lifecycle"
			$ gh search code react lifecycle

			# search code matching "error handling" 
			$ gh search code "error handling"
	
			# search code matching "deque" in Python files
			$ gh search code deque --language=python

			# search code matching "cli" in repositories owned by microsoft organization
			$ gh search code cli --owner=microsoft

			# search code matching "html" in repositories owned by octocat
			$ gh search code html --owner=octocat

			# search code matching "panic" in cli repository
			$ gh search code panic --repo cli/cli

			# search code matching keyword "lint" in package.json files
			$ gh search code lint --filename package.json
		`),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 && c.Flags().NFlag() == 0 {
				return cmdutil.FlagErrorf("specify search keywords or flags")
			}
			if opts.Query.Limit < 1 || opts.Query.Limit > shared.SearchMaxResults {
				return cmdutil.FlagErrorf("`--limit` must be between 1 and 1000")
			}
			opts.Query.Keywords = args
			if runF != nil {
				return runF(opts)
			}
			var err error
			opts.Searcher, err = shared.Searcher(f)
			if err != nil {
				return err
			}
			return codeRun(opts)
		},
	}

	// Output flags
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, search.CodeFields)
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the search query in the web browser")

	// Query parameter flags
	cmd.Flags().IntVarP(&opts.Query.Limit, "limit", "L", 30, "Maximum number of code results to fetch")

	// Query qualifier flags
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Extension, "extension", "", "Filter on file extension")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Filename, "filename", "", "Filter on filename")
	cmdutil.StringSliceEnumFlag(cmd, &opts.Query.Qualifiers.In, "match", "", nil, []string{"file", "path"}, "Restrict search to file contents or file path")
	cmd.Flags().StringVarP(&opts.Query.Qualifiers.Language, "language", "l", "", "Filter results by language")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.Repo, "repo", nil, "Filter on repository")
	cmd.Flags().StringVar(&opts.Query.Qualifiers.Size, "size", "", "Filter on size range, in kilobytes")
	cmd.Flags().StringSliceVar(&opts.Query.Qualifiers.User, "owner", nil, "Filter on owner")

	return cmd
}

func codeRun(opts *CodeOptions) error {
	io := opts.IO
	if opts.WebMode {
		url := opts.Searcher.URL(opts.Query)
		if io.IsStdoutTTY() {
			fmt.Fprintf(io.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(url))
		}
		return opts.Browser.Browse(url)
	}
	io.StartProgressIndicator()
	results, err := opts.Searcher.Code(opts.Query)
	io.StopProgressIndicator()
	if err != nil {
		return err
	}
	if len(results.Items) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError("no code results matched your search")
	}
	if err := io.StartPager(); err == nil {
		defer io.StopPager()
	} else {
		fmt.Fprintf(io.ErrOut, "failed to start pager: %v\n", err)
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(io, results)
	}

	return displayResults(io, results, opts.Query.Limit)
}

func displayResults(io *iostreams.IOStreams, results search.CodeResult, limit int) error {
	cs := io.ColorScheme()
	tp := tableprinter.New(io)
	displayed := 0
	for _, code := range results.Items {
		if displayed == limit {
			break
		}
		if io.IsStdoutTTY() {
			header := fmt.Sprintf("%s %s", cs.GreenBold(code.Repo.FullName), cs.GreenBold(code.Path))
			tp.AddField(header)
			tp.EndRow()
		}
		row := 0
		for _, textMatch := range code.TextMatches {
			out, shouldPrint := buildOutput(textMatch, cs)

			for i, line := range strings.Split(out, "\n") {
				if !shouldPrint[i] {
					continue
				}
				if displayed == limit {
					break
				}
				if io.IsStdoutTTY() {
					tp.AddField(fmt.Sprintf("%s: %s", cs.CyanBold(strconv.Itoa(row+1)), line))
				} else {
					tp.AddField(fmt.Sprintf("%s %s: %s", code.Repo.FullName, code.Path, line))
				}
				tp.EndRow()
				row++
				displayed++
			}
		}
	}
	if io.IsStdoutTTY() {
		header := fmt.Sprintf("Showing %d of %d results\n\n", displayed, max(displayed, results.Total))
		fmt.Fprintf(io.Out, "\n%s", header)
	}
	return tp.Render()
}

func buildOutput(tm search.TextMatch, cs *iostreams.ColorScheme) (string, map[int]bool) {
	shouldHighlight := getHighlightIndices(tm.Matches)
	linesToPrint := make(map[int]bool)
	line := 0
	var out strings.Builder
	for i, c := range tm.Fragment {
		if shouldHighlight[i] {
			out.WriteString(cs.CyanBold(string(c)))
			linesToPrint[line] = true
		} else {
			out.WriteRune(c)
		}
		if c == '\n' {
			line++
		}
	}
	return out.String(), linesToPrint
}

func getHighlightIndices(matches []search.Match) map[int]bool {
	m := make(map[int]bool)
	for _, match := range matches {
		start := match.Indices[0]
		stop := match.Indices[1]
		for i := start; i < stop; i++ {
			m[i] = true
		}
	}
	return m
}

func max(a, b int) int {
	if b < a {
		return a
	}
	return b
}
