package search

import (
	"net/http"
	"regexp"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type SearchOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)

	Pattern    *regexp.Regexp
	Filename   bool
	Code       bool
	Visibility string // all, secret, public
	Limit      int
}

func NewCmdSearch(f *cmdutil.Factory, runF func(*SearchOptions) error) *cobra.Command {
	opts := &SearchOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
	}

	var flagPublic bool
	var flagSecret bool

	cmd := &cobra.Command{
		Use:   "search <pattern>",
		Short: "Search your gists",
		Long: heredoc.Docf(`
			Search your gists' for the given POSIX regular expression.

			By default, all gists' descriptions are searched. Pass %[1]s--filename%[1]s to search
			file names, or %[1]s--code%[1]s to search content as well.
		`, "`"),
		Example: heredoc.Doc(`
			# search public gists' descriptions for "octo"
			$ gh gist search --public octo

			# search all gists' descriptions, file names, and code for "foo|bar"
			$ gh gist search --filename --code "foo|bar"
		`),
		Args: cmdutil.ExactArgs(1, "no search pattern passed"),
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			opts.Pattern, err = regexp.CompilePOSIX(args[0])
			if err != nil {
				return err
			}

			// Replicate precedence of existing `gist` commands instead of mutually exclusive arguments.
			opts.Visibility = "all"
			if flagSecret {
				opts.Visibility = "secret"
			} else if flagPublic {
				opts.Visibility = "public"
			}

			if runF != nil {
				return runF(opts)
			}
			return searchRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 10, "Maximum number of gists to fetch")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Show only public gists")
	cmd.Flags().BoolVar(&flagSecret, "secret", false, "Show only secret gists")
	cmd.Flags().BoolVar(&opts.Filename, "filename", false, "Include file names in search")
	cmd.Flags().BoolVar(&opts.Code, "code", false, "Include code in search")

	return cmd
}

func searchRun(opts *SearchOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	// Query as many as possible. Limit will apply to search results.
	allGists, err := shared.ListGists(client, host, shared.MaxPerPage, opts.Visibility, opts.Code)
	if err != nil {
		return err
	}

	gists := make([]shared.Gist, 0, opts.Limit)
	for _, gist := range allGists {
		if len(gists) == opts.Limit {
			break
		}

		if opts.Pattern.MatchString(gist.Description) {
			gists = append(gists, gist)
			continue
		}

		if opts.Filename || opts.Code {
			for _, file := range gist.Files {
				if opts.Filename && opts.Pattern.MatchString(file.Filename) {
					gists = append(gists, gist)
					continue
				}

				if opts.Code && opts.Pattern.MatchString(file.Content) {
					gists = append(gists, gist)
					continue
				}
			}
		}
	}

	if len(gists) == 0 {
		return cmdutil.NewNoResultsError("no gists found")
	}

	return shared.PrintGists(opts.IO, gists)
}
