package reopen

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ReopenOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg string
}

func NewCmdReopen(f *cmdutil.Factory, runF func(*ReopenOptions) error) *cobra.Command {
	opts := &ReopenOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "reopen {<number> | <url> | <branch>}",
		Short: "Reopen a pull request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return reopenRun(opts)
		},
	}

	return cmd
}

func reopenRun(opts *ReopenOptions) error {
	cs := opts.IO.ColorScheme()

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "number", "state", "title"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) can't be reopened because it was already merged\n", cs.FailureIcon(), pr.Number, pr.Title)
		return cmdutil.SilentError
	}

	if pr.IsOpen() {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) is already open\n", cs.WarningIcon(), pr.Number, pr.Title)
		return nil
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	err = api.PullRequestReopen(httpClient, baseRepo, pr.ID)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Reopened pull request #%d (%s)\n", cs.SuccessIconWithColor(cs.Green), pr.Number, pr.Title)

	return nil
}
