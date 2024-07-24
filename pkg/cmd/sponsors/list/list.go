package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Username string
	Sponsors []string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sponsors for a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Username = args[0]

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create http client: %w", err)
	}

	opts.Sponsors, err = api.GetSponsorsList(httpClient, "", opts.Username)
	if err != nil {
		return err
	}

	return formatTTYOutput(opts)
}

func formatTTYOutput(opts *ListOptions) error {

	table := tableprinter.New(opts.IO, tableprinter.WithHeader("SPONSORS"))
	for _, sponsor := range opts.Sponsors {
		table.AddField(sponsor)
		table.EndRow()
	}

	err := table.Render()
	if err != nil {
		return err
	}

	return nil
}
