package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	Exporter   cmdutil.Exporter
	Detector   fd.Detector

	Limit int
	Owner string

	Visibility  string
	Fork        bool
	Source      bool
	Language    string
	Topic       []string
	Archived    bool
	NonArchived bool

	Now func() time.Time
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}

	var (
		flagPublic  bool
		flagPrivate bool
	)

	cmd := &cobra.Command{
		Use:   "list [<owner>]",
		Args:  cobra.MaximumNArgs(1),
		Short: "List repositories owned by user or organization",
		Long: heredoc.Docf(`
			List repositories owned by a user or organization.

			Note that the list will only include repositories owned by the provided argument,
			and the %[1]s--fork%[1]s or %[1]s--source%[1]s flags will not traverse ownership boundaries. For example,
			when listing the forks in an organization, the output would not include those owned by individual users.
		`, "`"),
		Aliases: []string{"ls"},
		RunE: func(c *cobra.Command, args []string) error {
			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			if err := cmdutil.MutuallyExclusive("specify only one of `--public`, `--private`, or `--visibility`", flagPublic, flagPrivate, opts.Visibility != ""); err != nil {
				return err
			}
			if opts.Source && opts.Fork {
				return cmdutil.FlagErrorf("specify only one of `--source` or `--fork`")
			}
			if opts.Archived && opts.NonArchived {
				return cmdutil.FlagErrorf("specify only one of `--archived` or `--no-archived`")
			}

			if flagPrivate {
				opts.Visibility = "private"
			} else if flagPublic {
				opts.Visibility = "public"
			}

			if len(args) > 0 {
				opts.Owner = args[0]
			}

			if runF != nil {
				return runF(&opts)
			}
			return listRun(&opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of repositories to list")
	cmd.Flags().BoolVar(&opts.Source, "source", false, "Show only non-forks")
	cmd.Flags().BoolVar(&opts.Fork, "fork", false, "Show only forks")
	cmd.Flags().StringVarP(&opts.Language, "language", "l", "", "Filter by primary coding language")
	cmd.Flags().StringSliceVarP(&opts.Topic, "topic", "", nil, "Filter by topic")
	cmdutil.StringEnumFlag(cmd, &opts.Visibility, "visibility", "", "", []string{"public", "private", "internal"}, "Filter by repository visibility")
	cmd.Flags().BoolVar(&opts.Archived, "archived", false, "Show only archived repositories")
	cmd.Flags().BoolVar(&opts.NonArchived, "no-archived", false, "Omit archived repositories")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, api.RepositoryFields)

	cmd.Flags().BoolVar(&flagPrivate, "private", false, "Show only private repositories")
	cmd.Flags().BoolVar(&flagPublic, "public", false, "Show only public repositories")
	_ = cmd.Flags().MarkDeprecated("public", "use `--visibility=public` instead")
	_ = cmd.Flags().MarkDeprecated("private", "use `--visibility=private` instead")

	return cmd
}

var defaultFields = []string{"nameWithOwner", "description", "isPrivate", "isFork", "isArchived", "createdAt", "pushedAt"}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	if opts.Detector == nil {
		cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
		opts.Detector = fd.NewDetector(cachedClient, host)
	}
	features, err := opts.Detector.RepositoryFeatures()
	if err != nil {
		return err
	}

	fields := defaultFields
	if features.VisibilityField {
		fields = append(defaultFields, "visibility")
	}

	filter := FilterOptions{
		Visibility:  opts.Visibility,
		Fork:        opts.Fork,
		Source:      opts.Source,
		Language:    opts.Language,
		Topic:       opts.Topic,
		Archived:    opts.Archived,
		NonArchived: opts.NonArchived,
		Fields:      fields,
	}
	if opts.Exporter != nil {
		filter.Fields = opts.Exporter.Fields()
	}

	listResult, err := listRepos(httpClient, host, opts.Limit, opts.Owner, filter)
	if err != nil {
		return err
	}

	if opts.Owner != "" && listResult.Owner == "" && !listResult.FromSearch {
		return fmt.Errorf("the owner handle %q was not recognized as either a GitHub user or an organization", opts.Owner)
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, listResult.Repositories)
	}

	cs := opts.IO.ColorScheme()
	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("NAME", "DESCRIPTION", "INFO", "UPDATED"))

	totalMatchCount := len(listResult.Repositories)
	for _, repo := range listResult.Repositories {
		info := repoInfo(repo)
		infoColor := cs.Gray

		if repo.IsPrivate {
			infoColor = cs.Yellow
		}

		t := repo.PushedAt
		if repo.PushedAt == nil {
			t = &repo.CreatedAt
		}

		tp.AddField(repo.NameWithOwner, tableprinter.WithColor(cs.Bold))
		tp.AddField(text.RemoveExcessiveWhitespace(repo.Description))
		tp.AddField(info, tableprinter.WithColor(infoColor))
		tp.AddTimeField(opts.Now(), *t, cs.Gray)
		tp.EndRow()
	}

	if listResult.FromSearch && opts.Limit > 1000 {
		fmt.Fprintln(opts.IO.ErrOut, "warning: this query uses the Search API which is capped at 1000 results maximum")
	}
	if opts.IO.IsStdoutTTY() {
		hasFilters := filter.Visibility != "" || filter.Fork || filter.Source || filter.Language != "" || len(filter.Topic) > 0
		title := listHeader(listResult.Owner, totalMatchCount, listResult.TotalCount, hasFilters)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	if totalMatchCount > 0 {
		return tp.Render()
	}

	return nil
}

func listHeader(owner string, matchCount, totalMatchCount int, hasFilters bool) string {
	if totalMatchCount == 0 {
		if hasFilters {
			return "No results match your search"
		} else if owner != "" {
			return "There are no repositories in @" + owner
		}
		return "No results"
	}

	var matchStr string
	if hasFilters {
		matchStr = " that match your search"
	}
	return fmt.Sprintf("Showing %d of %d repositories in @%s%s", matchCount, totalMatchCount, owner, matchStr)
}

func repoInfo(r api.Repository) string {
	tags := []string{visibilityLabel(r)}

	if r.IsFork {
		tags = append(tags, "fork")
	}
	if r.IsArchived {
		tags = append(tags, "archived")
	}

	return strings.Join(tags, ", ")
}

func visibilityLabel(repo api.Repository) string {
	if repo.Visibility != "" {
		return strings.ToLower(repo.Visibility)
	} else if repo.IsPrivate {
		return "private"
	}
	return "public"
}
