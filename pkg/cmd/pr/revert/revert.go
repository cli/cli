package revert

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type RevertOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg string

	Body    string
	BodySet bool
	Title   string
	IsDraft bool
}

func NewCmdRevert(f *cmdutil.Factory, runF func(*RevertOptions) error) *cobra.Command {
	opts := &RevertOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	var bodyFile string

	cmd := &cobra.Command{
		Use:   "revert {<number> | <url> | <branch>}",
		Short: "Revert a pull request",
		Args:  cmdutil.ExactArgs(1, "cannot revert pull request: number, url, or branch required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			bodyProvided := cmd.Flags().Changed("body")
			bodyFileProvided := bodyFile != ""

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--body` or `--body-file`",
				bodyProvided,
				bodyFileProvided,
			); err != nil {
				return err
			}

			if bodyProvided || bodyFileProvided {
				opts.BodySet = true
				if bodyFileProvided {
					b, err := cmdutil.ReadFile(bodyFile, opts.IO.In)
					if err != nil {
						return err
					}
					opts.Body = string(b)
				}
			}

			if runF != nil {
				return runF(opts)
			}
			return revertRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.IsDraft, "draft", "d", false, "Mark revert pull request as a draft")
	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title for the revert pull request")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Body for the revert pull request")
	cmd.Flags().StringVarP(&bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" to read from standard input)")
	return cmd
}

func revertRun(opts *RevertOptions) error {
	cs := opts.IO.ColorScheme()

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"id", "number", "state", "title"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}
	if pr.State != "MERGED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request %s#%d (%s) can't be reverted because it has not been merged\n", cs.FailureIcon(), ghrepo.FullName(baseRepo), pr.Number, pr.Title)
		return cmdutil.SilentError
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	params := githubv4.RevertPullRequestInput{
		PullRequestID: pr.ID,
		Title:         githubv4.NewString(githubv4.String(opts.Title)),
		Body:          githubv4.NewString(githubv4.String(opts.Body)),
		Draft:         githubv4.NewBoolean(githubv4.Boolean(opts.IsDraft)),
	}

	revertPR, err := api.PullRequestRevert(apiClient, baseRepo, params)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(
		opts.IO.ErrOut,
		"%s Created pull request %s#%d (%s) that reverts %s#%d (%s)\n",
		cs.SuccessIconWithColor(cs.Green),
		ghrepo.FullName(baseRepo),
		revertPR.Number,
		revertPR.Title,
		ghrepo.FullName(baseRepo),
		pr.Number,
		pr.Title,
	)

	return nil
}
