package ready

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ReadyOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg string
	Undo        bool
}

func NewCmdReady(f *cmdutil.Factory, runF func(*ReadyOptions) error) *cobra.Command {
	opts := &ReadyOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "ready [<number> | <url> | <branch>]",
		Short: "Mark a pull request as ready for review",
		Long: heredoc.Doc(`
			Mark a pull request as ready for review.

			Without an argument, the pull request that belongs to the current branch
			is marked as ready.

			If supported by your plan, convert to draft with --undo
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the --repo flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return readyRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Undo, "undo", false, `Convert a pull request to "draft"`)
	return cmd
}

func readyRun(opts *ReadyOptions) error {
	cs := opts.IO.ColorScheme()

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "number", "state", "isDraft"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	if !pr.IsOpen() {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is closed. Only draft pull requests can be marked as \"ready for review\"\n", cs.FailureIcon(), pr.Number)
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	if opts.Undo { // convert to draft
		if pr.IsDraft {
			fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is already \"in draft\"\n", cs.WarningIcon(), pr.Number)
			return nil
		}
		err = api.ConvertPullRequestToDraft(apiClient, baseRepo, pr)
		if err != nil {
			return fmt.Errorf("API call failed: %w", err)
		}

		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is converted to \"draft\"\n", cs.SuccessIconWithColor(cs.Green), pr.Number)
	} else { // mark as ready for review
		if !pr.IsDraft {
			fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is already \"ready for review\"\n", cs.WarningIcon(), pr.Number)
			return nil
		}

		err = api.PullRequestReady(apiClient, baseRepo, pr)
		if err != nil {
			return fmt.Errorf("API call failed: %w", err)
		}

		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d is marked as \"ready for review\"\n", cs.SuccessIconWithColor(cs.Green), pr.Number)
	}

	return nil
}
