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

	Branch string
	Base   string
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
	cmd.Flags().StringVar(&opts.Branch, "branch", "", "Name of the branch to create for the revert pull request")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "Base branch for the revert pull request")
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
		Draft:         new(githubv4.Boolean(opts.IsDraft)),
	}
	// Only set the Body field when opts.BodySet is true to avoid overriding
	// GitHub's default revert body generation.
	if opts.BodySet {
		params.Body = new(githubv4.String(opts.Body))
	}
	// Only set the Title field when opts.Title is not empty to avoid overriding
	// GitHub's default revert title generation.
	if opts.Title != "" {
		params.Title = new(githubv4.String(opts.Title))
	}

	revertPR, err := api.PullRequestRevert(apiClient, baseRepo, params)
	if err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "%s %s\n", cs.FailureIcon(), err)
		return fmt.Errorf("API call failed: %w", err)
	}

	if revertPR != nil {
		// Print the URL before the follow-up steps so the reference to the
		// revert pull request is always available, even if one of them fails.
		fmt.Fprintln(opts.IO.Out, revertPR.URL)

		// Rename the branch that GitHub created for the revert, if requested.
		if opts.Branch != "" {
			if revertPR.HeadRefName == "" {
				fmt.Fprintf(opts.IO.ErrOut, "%s could not rename branch: revert PR has no head branch name\n", cs.WarningIcon())
			} else if _, err := api.RenameBranch(apiClient, baseRepo, revertPR.HeadRefName, opts.Branch); err != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s Failed to rename branch %s to %s: %s\n", cs.FailureIcon(), revertPR.HeadRefName, opts.Branch, err)
				return fmt.Errorf("branch rename failed: %w", err)
			}
		}

		// Change the base branch of the revert pull request, if requested.
		if opts.Base != "" {
			if revertPR.ID == "" {
				fmt.Fprintf(opts.IO.ErrOut, "%s could not change base branch: revert PR has no ID\n", cs.WarningIcon())
			} else if err := api.UpdatePullRequestBase(apiClient, baseRepo, revertPR.ID, opts.Base); err != nil {
				fmt.Fprintf(opts.IO.ErrOut, "%s Failed to change base branch to %s: %s\n", cs.FailureIcon(), opts.Base, err)
				return fmt.Errorf("base branch update failed: %w", err)
			}
		}
	}
	return nil
}
