package list

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Getter     *SponsorsListGetter
	Prompter   prompter.Prompter

	Username string
	Sponsors []string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		Getter:     NewSponsorsListGetter(getSponsorsList),
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sponsors for a user",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			if len(args) == 0 {
				input, err := opts.Prompter.Input("Which user do you want to target?", "")
				if err != nil {
					return fmt.Errorf("could not prompt")
				}
				opts.Username = input
			} else {
				opts.Username = args[0]
			}

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

	opts.Sponsors, err = opts.Getter.get(httpClient, "", opts.Username)
	if err != nil {
		return err
	}

	return formatOutput(opts)
}

func formatOutput(opts *ListOptions) error {

	if len(opts.Sponsors) == 0 && opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "No sponsors found for %s\n", opts.Username)
		return nil
	}

	table := tableprinter.New(opts.IO, tableprinter.WithHeader("SPONSORS"))
	for _, sponsor := range opts.Sponsors {
		table.AddField(sponsor)
		table.EndRow()
	}

	return table.Render()
}
