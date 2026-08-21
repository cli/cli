package list

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

var jsonFields = []string{"id", "name", "dataType", "options"}

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Exporter   cmdutil.Exporter
	Detector   fd.Detector
}

// NewCmdList creates the issue field list command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issue fields in a repository",
		Example: heredoc.Doc(`
			$ gh issue field list
			$ gh issue field list --repo OWNER/REPO
			$ gh issue field list --json name,dataType,options
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, jsonFields)
	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	if opts.Detector == nil {
		cachedClient := api.NewCachedHTTPClient(httpClient, 24*time.Hour)
		opts.Detector = fd.NewDetector(cachedClient, repo.RepoHost())
	}
	issueFeatures, err := opts.Detector.IssueFeatures()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	fields, err := api.RepoIssueFields(api.NewClientFromHTTP(httpClient), repo, issueFeatures.IssueFieldMultiSelectSupported)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}
	if len(fields) == 0 && opts.Exporter == nil {
		return cmdutil.NewNoResultsError(fmt.Sprintf("no issue fields found in %s", ghrepo.FullName(repo)))
	}
	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, fields)
	}

	table := tableprinter.New(opts.IO, tableprinter.WithHeader("NAME", "DATA TYPE", "OPTIONS"))
	for _, field := range fields {
		optionNames := make([]string, len(field.Options))
		for i, option := range field.Options {
			optionNames[i] = option.Name
		}
		table.AddField(field.Name)
		table.AddField(strings.ToLower(field.DataType))
		table.AddField(strings.Join(optionNames, ", "))
		table.EndRow()
	}
	return table.Render()
}
