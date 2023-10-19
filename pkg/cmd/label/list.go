package label

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type listOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Exporter cmdutil.Exporter
	Query    listQueryOptions
	WebMode  bool
}

func newCmdList(f *cmdutil.Factory, runF func(*listOptions) error) *cobra.Command {
	opts := listOptions{
		Browser:    f.Browser,
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List labels in a repository",
		Long: heredoc.Docf(`
			Display labels in a GitHub repository.

			When using the %[1]s--search%[1]s flag results are sorted by best match of the query.
			This behavior cannot be configured with the %[1]s--order%[1]s or %[1]s--sort%[1]s flags.
		`, "`"),
		Example: heredoc.Doc(`
			# sort labels by name
			$ gh label list --sort name

			# find labels with "bug" in the name or description
			$ gh label list --search bug
		`),
		Args:    cobra.NoArgs,
		Aliases: []string{"ls"},
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.Query.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Query.Limit)
			}

			if opts.Query.Query != "" &&
				(c.Flags().Changed("order") || c.Flags().Changed("sort")) {
				return cmdutil.FlagErrorf("cannot specify `--order` or `--sort` with `--search`")
			}

			if runF != nil {
				return runF(&opts)
			}
			return listRun(&opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List labels in the web browser")
	cmd.Flags().IntVarP(&opts.Query.Limit, "limit", "L", 30, "Maximum number of labels to fetch")
	cmd.Flags().StringVarP(&opts.Query.Query, "search", "S", "", "Search label names and descriptions")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Order, "order", "", defaultOrder, []string{"asc", "desc"}, "Order of labels returned")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Sort, "sort", "", defaultSort, []string{"created", "name"}, "Sort fetched labels")

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, labelFields)

	return cmd
}

func listRun(opts *listOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.WebMode {
		labelListURL := ghrepo.GenerateRepoURL(baseRepo, "labels")

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(labelListURL))
		}

		return opts.Browser.Browse(labelListURL)
	}

	if opts.Exporter != nil {
		opts.Query.fields = opts.Exporter.Fields()
	}

	opts.IO.StartProgressIndicator()
	labels, totalCount, err := listLabels(httpClient, baseRepo, opts.Query)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if len(labels) == 0 {
		if opts.Query.Query != "" {
			return cmdutil.NewNoResultsError(fmt.Sprintf("no labels in %s matched your search", ghrepo.FullName(baseRepo)))
		}
		return cmdutil.NewNoResultsError(fmt.Sprintf("no labels found in %s", ghrepo.FullName(baseRepo)))
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, labels)
	}

	if opts.IO.IsStdoutTTY() {
		title := listHeader(ghrepo.FullName(baseRepo), len(labels), totalCount)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	return printLabels(opts.IO, labels)
}

func printLabels(io *iostreams.IOStreams, labels []label) error {
	cs := io.ColorScheme()
	table := tableprinter.New(io, tableprinter.WithHeader("NAME", "DESCRIPTION", "COLOR"))

	for _, label := range labels {
		table.AddField(label.Name, tableprinter.WithColor(cs.ColorFromRGB(label.Color)))
		table.AddField(label.Description)
		table.AddField("#" + label.Color)

		table.EndRow()
	}

	return table.Render()
}

func listHeader(repoName string, count int, totalCount int) string {
	return fmt.Sprintf("Showing %d of %s in %s", count, text.Pluralize(totalCount, "label"), repoName)
}
