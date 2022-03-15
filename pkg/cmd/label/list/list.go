package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
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
			if runF != nil {
				return runF(&opts)
			}
			return listRun(&opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List labels in the web browser")

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

	labels, err := getLabels(client, baseRepo)
	if err != nil {
		return err
	}

	opts.IO.StopProgressIndicator()

	printLabels(labels, opts.IO)

	return nil
}

type Label struct {
	Id          int
	Name        string
	Color       string
	Default     bool
	Url         string
	Description string
}

func getLabels(client *api.Client, repo ghrepo.Interface) ([]Label, error) {
	labels := []Label{}

	path := fmt.Sprintf("repos/%s/%s/labels", repo.RepoOwner(), repo.RepoName())

	err := client.REST(repo.RepoHost(), "GET", path, nil, &labels)
	if err != nil {
		return nil, err
	}
	return labels, nil
}

func printLabels(labels []Label, io *iostreams.IOStreams) error {
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
