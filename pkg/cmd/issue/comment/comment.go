package comment

import (
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	issueShared "github.com/cli/cli/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func NewCmdComment(f *cmdutil.Factory, runF func(*prShared.CommentableOptions) error) *cobra.Command {
	opts := &prShared.CommentableOptions{
		IO:                    f.IOStreams,
		HttpClient:            f.HttpClient,
		EditSurvey:            prShared.CommentableEditSurvey(f.Config, f.IOStreams),
		InteractiveEditSurvey: prShared.CommentableInteractiveEditSurvey(f.Config, f.IOStreams),
		ConfirmSubmitSurvey:   prShared.CommentableConfirmSubmitSurvey,
		OpenInBrowser:         utils.OpenInBrowser,
	}

	cmd := &cobra.Command{
		Use:   "comment {<number> | <url>}",
		Short: "Create a new issue comment",
		Example: heredoc.Doc(`
			$ gh issue comment 22 --body "I was able to reproduce this issue, lets fix it."
		`),
		Args: cobra.ExactArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.RetrieveCommentable = retrieveIssue(f.HttpClient, f.BaseRepo, args[0])
			return prShared.CommentablePreRun(cmd, opts)
		},
		RunE: func(_ *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return prShared.CommentableRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Supply a body. Will prompt for one otherwise.")
	cmd.Flags().BoolP("editor", "e", false, "Add body using editor")
	cmd.Flags().BoolP("web", "w", false, "Add body in browser")

	return cmd
}

func retrieveIssue(httpClient func() (*http.Client, error),
	baseRepo func() (ghrepo.Interface, error),
	selector string) func() (prShared.Commentable, ghrepo.Interface, error) {
	return func() (prShared.Commentable, ghrepo.Interface, error) {
		httpClient, err := httpClient()
		if err != nil {
			return nil, nil, err
		}
		apiClient := api.NewClientFromHTTP(httpClient)

		issue, repo, err := issueShared.IssueFromArg(apiClient, baseRepo, selector)
		if err != nil {
			return nil, nil, err
		}

		return issue, repo, nil
	}
}
