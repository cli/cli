package list

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)

	Limit          int
	Filter         *regexp.Regexp
	IncludeContent bool
	Visibility     string // all, secret, public
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	var flagPublic bool
	var flagSecret bool
	var flagFilter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your gists",
		Long: heredoc.Docf(`
			List gists from your user account.

			You can use a regular expression to filter the description, file names,
			or even the content of files in the gist using %[1]s--filter%[1]s.

			For supported regular expression syntax, see <https://pkg.go.dev/regexp/syntax>.

			Use %[1]s--include-content%[1]s to include content of files, noting that
			this will be slower and increase the rate limit used. Instead of printing a table,
			code will be printed with highlights similar to %[1]sgh search code%[1]s:

				{{gist ID}} {{file name}}
				    {{description}}
				        {{matching lines from content}}

			No highlights or other color is printed when output is redirected.
		`, "`"),
		Example: heredoc.Doc(`
			# list all secret gists from your user account
			$ gh gist list --secret

			# find all gists from your user account mentioning "octo" anywhere
			$ gh gist list --filter octo --include-content
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			if flagFilter == "" {
				if opts.IncludeContent {
					return cmdutil.FlagErrorf("cannot use --include-content without --filter")
				}
			} else {
				if filter, err := regexp.CompilePOSIX(flagFilter); err != nil {
					return err
				} else {
					opts.Filter = filter
				}
			}

			opts.Visibility = "all"
			if flagSecret {
				opts.Visibility = "secret"
			} else if flagPublic {
				opts.Visibility = "public"
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 10, "Maximum number of gists to fetch")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Show only public gists")
	cmd.Flags().BoolVar(&flagSecret, "secret", false, "Show only secret gists")
	cmd.Flags().StringVar(&flagFilter, "filter", "", "Filter gists using a regular `expression`")
	cmd.Flags().BoolVar(&opts.IncludeContent, "include-content", false, "Include gists' file content when filtering")

	return cmd
}

func listRun(opts *ListOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	// Filtering can take a while so start the progress indicator. StopProgressIndicator will no-op if not running.
	if opts.Filter != nil {
		opts.IO.StartProgressIndicator()
	}
	gists, err := shared.ListGists(client, host, opts.Limit, opts.Filter, opts.IncludeContent, opts.Visibility)
	opts.IO.StopProgressIndicator()

	if err != nil {
		return err
	}

	if len(gists) == 0 {
		return cmdutil.NewNoResultsError("no gists found")
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.Filter != nil && opts.IncludeContent {
		return printContent(opts.IO, gists, opts.Filter)
	}

	return printTable(opts.IO, gists, opts.Filter)
}

func printTable(io *iostreams.IOStreams, gists []shared.Gist, filter *regexp.Regexp) error {
	cs := io.ColorScheme()
	tp := tableprinter.New(io, tableprinter.WithHeader("ID", "DESCRIPTION", "FILES", "VISIBILITY", "UPDATED"))

	// Highlight filter matches in the description when printing the table.
	highlightDescription := func(s string) string {
		if filter != nil {
			if str, err := highlightMatch(s, filter, nil, cs.Bold, cs.Highlight); err == nil {
				return str
			}
		}
		return cs.Bold(s)
	}

	// Highlight the files column when any file name matches the filter.
	highlightFilesFunc := func(gist *shared.Gist) func(string) string {
		if filter != nil {
			for _, file := range gist.Files {
				if filter.MatchString(file.Filename) {
					return cs.Highlight
				}
			}
		}
		return normal
	}

	for _, gist := range gists {
		fileCount := len(gist.Files)

		visibility := "public"
		visColor := cs.Green
		if !gist.Public {
			visibility = "secret"
			visColor = cs.Red
		}

		description := gist.Description
		if description == "" {
			for filename := range gist.Files {
				if !strings.HasPrefix(filename, "gistfile") {
					description = filename
					break
				}
			}
		}

		tp.AddField(gist.ID)
		tp.AddField(
			text.RemoveExcessiveWhitespace(description),
			tableprinter.WithColor(highlightDescription),
		)
		tp.AddField(
			text.Pluralize(fileCount, "file"),
			tableprinter.WithColor(highlightFilesFunc(&gist)),
		)
		tp.AddField(visibility, tableprinter.WithColor(visColor))
		tp.AddTimeField(time.Now(), gist.UpdatedAt, cs.Gray)
		tp.EndRow()
	}

	return tp.Render()
}

// printContent prints a gist with optional description and content similar to `gh search code`
// including highlighted matches in the form:
//
//	{{gist ID}} {{file name}}
//	    {{description, if any}}
//	        {{content lines with matches, if any}}
//
// If printing to a non-TTY stream the format will be the same but without highlights.
func printContent(io *iostreams.IOStreams, gists []shared.Gist, filter *regexp.Regexp) error {
	const tab string = "    "
	cs := io.ColorScheme()

	out := &strings.Builder{}
	var filename, description string
	var err error
	split := func(r rune) bool {
		return r == '\n' || r == '\r'
	}

	for _, gist := range gists {
		for _, file := range gist.Files {
			matched := false
			out.Reset()

			if filename, err = highlightMatch(file.Filename, filter, &matched, cs.Green, cs.Highlight); err != nil {
				return err
			}
			fmt.Fprintln(out, cs.Blue(gist.ID), filename)

			if gist.Description != "" {
				if description, err = highlightMatch(gist.Description, filter, &matched, cs.Bold, cs.Highlight); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s%s\n", tab, description)
			}

			if file.Content != "" {
				for _, line := range strings.FieldsFunc(file.Content, split) {
					if filter.MatchString(line) {
						if line, err = highlightMatch(line, filter, &matched, normal, cs.Highlight); err != nil {
							return err
						}
						fmt.Fprintf(out, "%[1]s%[1]s%[2]s\n", tab, line)
					}
				}
			}

			if matched {
				fmt.Fprintln(io.Out, out.String())
			}
		}
	}

	return nil
}

func highlightMatch(s string, filter *regexp.Regexp, matched *bool, color, highlight func(string) string) (string, error) {
	matches := filter.FindAllStringIndex(s, -1)
	if matches == nil {
		return color(s), nil
	}

	out := strings.Builder{}

	// Color up to the first match. If an empty string, no ANSI color sequence is added.
	if _, err := out.WriteString(color(s[:matches[0][0]])); err != nil {
		return "", err
	}

	// Highlight each match, then color the remaining text which, if an empty string, no ANSI color sequence is added.
	for i, match := range matches {
		if _, err := out.WriteString(highlight(s[match[0]:match[1]])); err != nil {
			return "", err
		}

		text := s[match[1]:]
		if i+1 < len(matches) {
			text = s[match[1]:matches[i+1][0]]
		}
		if _, err := out.WriteString(color(text)); err != nil {
			return "", nil
		}
	}

	if matched != nil {
		*matched = *matched || true
	}

	return out.String(), nil
}

func normal(s string) string {
	return s
}
