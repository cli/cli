package label

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type listOptions struct {
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Query   listQueryOptions
	WebMode bool
}

type listQueryOptions struct {
	Limit int
	Query string
	Order string
	Sort  string
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

		You cannot use %[1]s--search%[1]s with either %[1]s--order%[1]s or %[1]s--sort%[1]s.
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
	cmd.Flags().IntVarP(&opts.Query.Limit, "limit", "L", 30, "Maximum number of items to fetch")
	cmd.Flags().StringVarP(&opts.Query.Query, "search", "S", "", "Search label names and descriptions")

	// These defaults match the service behavior and, therefore, the initially release of `label list` behavior.
	cmdutil.StringEnumFlag(cmd, &opts.Query.Order, "order", "", "asc", []string{"asc", "desc"}, "Order of labels returned")
	cmdutil.StringEnumFlag(cmd, &opts.Query.Sort, "sort", "", "created", []string{"created", "name"}, "Sort fetched labels")

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
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(labelListURL))
		}

		return opts.Browser.Browse(labelListURL)
	}

	opts.IO.StartProgressIndicator()
	labels, totalCount, err := listLabels(httpClient, baseRepo, opts.Query)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if len(labels) == 0 {
		return cmdutil.NewNoResultsError(fmt.Sprintf("there are no labels in %s", ghrepo.FullName(baseRepo)))
	}

	if opts.IO.IsStdoutTTY() {
		title := listHeader(ghrepo.FullName(baseRepo), len(labels), totalCount)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	return printLabels(opts.IO, labels)
}

func printLabels(io *iostreams.IOStreams, labels []label) error {
	cs := io.ColorScheme()
	table := utils.NewTablePrinter(io)

	for _, label := range labels {
		table.AddField(label.Name, nil, cs.ColorFromRGB(label.Color))
		table.AddField(label.Description, text.Truncate, nil)
		table.AddField("#"+label.Color, nil, nil)

		table.EndRow()
	}

	return table.Render()
}

func listHeader(repoName string, count int, totalCount int) string {
	return fmt.Sprintf("Showing %d of %s in %s", count, utils.Pluralize(totalCount, "label"), repoName)
}

func (opts listQueryOptions) OrderBy() map[string]interface{} {
	field := strings.ToUpper(opts.Sort)
	if opts.Sort == "created" {
		field = "CREATED_AT"
	}
	return map[string]interface{}{
		"direction": strings.ToUpper(opts.Order),
		"field":     field,
	}
}
