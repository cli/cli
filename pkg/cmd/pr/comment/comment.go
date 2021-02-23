package comment

import (
	"errors"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, runF func(*shared.CommentableOptions) error) *cobra.Command {
	opts := &shared.CommentableOptions{
		IO:                    f.IOStreams,
		HttpClient:            f.HttpClient,
		EditSurvey:            shared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey: shared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:   shared.CommentableConfirmSubmitSurvey,
		OpenInBrowser:         utils.OpenInBrowser,
	}

	cmd := &cobra.Command{
		Use:   "comment [<number> | <url> | <branch>]",
		Short: "Create a new pr comment",
		Example: heredoc.Doc(`
			$ gh pr comment 22 --body "This looks great, lets get it deployed."
		`),
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return &cmdutil.FlagError{Err: errors.New("argument required when using the --repo flag")}
			}
			var selector string
			if len(args) > 0 {
				selector = args[0]
			}
			opts.RetrieveCommentable = retrievePR(f.HttpClient, f.BaseRepo, f.Branch, f.Remotes, selector)
			return shared.CommentablePreRun(cmd, opts)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return shared.CommentableRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	cmd.Flags().BoolP("editor", "e", false, "Add body using editor")
	cmd.Flags().BoolP("web", "w", false, "Add body in browser")

	return cmd
}

func retrievePR(httpClient func() (*http.Client, error),
	baseRepo func() (ghrepo.Interface, error),
	branch func() (string, error),
	remotes func() (context.Remotes, error),
	selector string) func() (shared.Commentable, ghrepo.Interface, error) {
	return func() (shared.Commentable, ghrepo.Interface, error) {
		httpClient, err := httpClient()
		if err != nil {
			return nil, nil, err
		}
		apiClient := api.NewClientFromHTTP(httpClient)

		pr, repo, err := shared.PRFromArgs(apiClient, baseRepo, branch, remotes, selector)
		if err != nil {
			return nil, nil, err
		}

		return pr, repo, nil
	}
}
