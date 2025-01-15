package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/cache/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Exporter   cmdutil.Exporter
	Now        time.Time

	Limit int
	Order string
	Sort  string
	Key   string
	Ref   string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List GitHub Actions caches",
		Example: heredoc.Doc(`
		# List caches for current repository
		$ gh cache list

		# List caches for specific repository
		$ gh cache list --repo cli/cli

		# List caches sorted by least recently accessed
		$ gh cache list --sort last_accessed_at --order asc

		# List caches that have keys matching a prefix (or that match exactly)
		$ gh cache list --key key-prefix

		# To list caches for a specific branch, replace <branch-name> with the actual branch name
		$ gh cache list --ref refs/heads/<branch-name>

		# To list caches for a specific pull request, replace <pr-number> with the actual pull request number
		$ gh cache list --ref refs/pull/<pr-number>/merge
		`),
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			if runF != nil {
				return runF(&opts)
			}

			return listRun(&opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of caches to fetch")
	cmdutil.StringEnumFlag(cmd, &opts.Order, "order", "O", "desc", []string{"asc", "desc"}, "Order of caches returned")
	cmdutil.StringEnumFlag(cmd, &opts.Sort, "sort", "S", "last_accessed_at", []string{"created_at", "last_accessed_at", "size_in_bytes"}, "Sort fetched caches")
	cmd.Flags().StringVarP(&opts.Key, "key", "k", "", "Filter by cache key prefix")
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "Filter by ref, formatted as refs/heads/<branch name> or refs/pull/<number>/merge")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.CacheFields)

	return cmd
}

func listRun(opts *ListOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(httpClient)

	opts.IO.StartProgressIndicator()
	result, err := shared.GetCaches(client, repo, shared.GetCachesOptions{Limit: opts.Limit, Sort: opts.Sort, Order: opts.Order, Key: opts.Key, Ref: opts.Ref})
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("%s Failed to get caches: %w", opts.IO.ColorScheme().FailureIcon(), err)
	}

	if len(result.ActionsCaches) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError(fmt.Sprintf("No caches found in %s", ghrepo.FullName(repo)))
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.Out, "Failed to start pager: %v\n", err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, result.ActionsCaches)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "\nShowing %d of %s in %s\n\n", len(result.ActionsCaches), text.Pluralize(result.TotalCount, "cache"), ghrepo.FullName(repo))
	}

	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}

	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("ID", "KEY", "SIZE", "CREATED", "ACCESSED"))
	for _, cache := range result.ActionsCaches {
		tp.AddField(opts.IO.ColorScheme().Cyan(fmt.Sprintf("%d", cache.Id)))
		tp.AddField(cache.Key)
		tp.AddField(humanFileSize(cache.SizeInBytes))
		tp.AddTimeField(opts.Now, cache.CreatedAt, opts.IO.ColorScheme().Gray)
		tp.AddTimeField(opts.Now, cache.LastAccessedAt, opts.IO.ColorScheme().Gray)
		tp.EndRow()
	}

	return tp.Render()
}

func humanFileSize(s int64) string {
	if s < 1024 {
		return fmt.Sprintf("%d B", s)
	}

	kb := float64(s) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%s KiB", floatToString(kb, 2))
	}

	mb := kb / 1024
	if mb < 1024 {
		return fmt.Sprintf("%s MiB", floatToString(mb, 2))
	}

	gb := mb / 1024
	return fmt.Sprintf("%s GiB", floatToString(gb, 2))
}

func floatToString(f float64, p uint8) string {
	fs := fmt.Sprintf("%#f%0*s", f, p, "")
	idx := strings.IndexRune(fs, '.')
	return fs[:idx+int(p)+1]
}
