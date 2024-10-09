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
			or even the content of files in the gist. See https://pkg.go.dev/regexp/syntax
			for the regular expression syntax you can pass to %[1]s--filter%[1]s. Pass
			%[1]s--include-content%[1]s to also search the content of files noting that
			this will take longer since all files' content is fetched.
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

			if filter, err := regexp.CompilePOSIX(flagFilter); err != nil {
				return err
			} else {
				opts.Filter = filter
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

	gists, err := shared.ListGists(client, host, opts.Limit, opts.Filter, opts.IncludeContent, opts.Visibility)
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

	cs := opts.IO.ColorScheme()
	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("ID", "DESCRIPTION", "FILES", "VISIBILITY", "UPDATED"))

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
			tableprinter.WithColor(cs.Bold),
		)
		tp.AddField(text.Pluralize(fileCount, "file"))
		tp.AddField(visibility, tableprinter.WithColor(visColor))
		tp.AddTimeField(time.Now(), gist.UpdatedAt, cs.Gray)
		tp.EndRow()
	}

	return tp.Render()
}
