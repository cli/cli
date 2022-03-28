package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/label/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser

	WebMode bool
	Limit   int
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "Show all labels",
		Long:    "Display all labels of a GitHub repository.",
		Args:    cobra.NoArgs,
		Aliases: []string{"ls"},
		RunE: func(c *cobra.Command, args []string) error {
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

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List labels in the web browser")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of items to fetch")

	return cmd
}

func listRun(opts *ListOptions) error {
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

	client := api.NewClientFromHTTP(httpClient)

	opts.IO.StartProgressIndicator()

	labels, totalCount, err := listLabels(client, baseRepo, opts.Limit)
	if err != nil {
		return err
	}

	opts.IO.StopProgressIndicator()

	if opts.IO.IsStdoutTTY() {
		title := listHeader(ghrepo.FullName(baseRepo), "label", len(labels), totalCount)
		fmt.Fprintf(opts.IO.Out, "\n%s\n\n", title)
	}

	printLabels(labels, opts.IO)

	return nil
}

func printLabels(labels []shared.Label, io *iostreams.IOStreams) error {
	cs := io.ColorScheme()
	table := utils.NewTablePrinter(io)

	for _, label := range labels {
		labelName := ""
		if table.IsTTY() {
			labelName = cs.HexToRGB(label.Color, label.Name)
		} else {
			labelName = label.Name
		}

		table.AddField(labelName, nil, nil)
		table.AddField(label.Description, nil, nil)

		table.EndRow()
	}

	return table.Render()
}

func listHeader(repoName string, itemName string, count int, totalCount int) string {
	if totalCount == 0 {
		return fmt.Sprintf("There are no %ss in %s", itemName, repoName)
	}

	return fmt.Sprintf("Showing %d of %s in %s", count, utils.Pluralize(totalCount, itemName), repoName)
}
