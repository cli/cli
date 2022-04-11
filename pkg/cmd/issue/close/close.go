package close

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/spf13/cobra"
)

type CloseOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string
	Comment     string
	Reason      string
}

func NewCmdClose(f *cmdutil.Factory, runF func(*CloseOptions) error) *cobra.Command {
	var completed, notPlanned bool

	opts := &CloseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "close {<number> | <url>}",
		Short: "Close issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive(
				"specify only one of '--completed' or '--not-planned'",
				completed,
				notPlanned,
			); err != nil {
				return err
			}

			if completed {
				opts.Reason = "COMPLETED"
			}
			if notPlanned {
				opts.Reason = "NOT_PLANNED"
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return closeRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "Leave a closing comment")
	cmd.Flags().BoolVar(&completed, "completed", false, "Close issue with completed reason")
	cmd.Flags().BoolVar(&notPlanned, "not-planned", false, "Close issue with not planned reason")

	return cmd
}

func closeRun(opts *CloseOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	issue, baseRepo, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, []string{"id", "number", "title", "state"})
	if err != nil {
		return err
	}

	if issue.State == "CLOSED" && opts.Reason == "" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue #%d (%s) is already closed\n", cs.Yellow("!"), issue.Number, issue.Title)
		return nil
	}

	if issue.IsPullRequest() && opts.Reason != "" {
		return errors.New("can not close pull request with reason")
	}

	if opts.Comment != "" {
		commentOpts := &prShared.CommentableOptions{
			Body:       opts.Comment,
			HttpClient: opts.HttpClient,
			InputType:  prShared.InputTypeInline,
			Quiet:      true,
			RetrieveCommentable: func() (prShared.Commentable, ghrepo.Interface, error) {
				return issue, baseRepo, nil
			},
		}
		err := prShared.CommentableRun(commentOpts)
		if err != nil {
			return err
		}
	}

	err = apiClose(httpClient, baseRepo, issue, opts.Reason)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Closed issue #%d (%s)\n", cs.SuccessIconWithColor(cs.Red), issue.Number, issue.Title)

	return nil
}

func apiClose(httpClient *http.Client, repo ghrepo.Interface, issue *api.Issue, reason string) error {
	if issue.IsPullRequest() {
		return api.PullRequestClose(httpClient, repo, issue.ID)
	}

	var mutation struct {
		CloseIssue struct {
			Issue struct {
				ID string
			}
		} `graphql:"closeIssue(input: $input)"`
	}

	type CloseIssueInput struct {
		IssueID     string `json:"issueId"`
		StateReason string `json:"stateReason"`
	}

	variables := map[string]interface{}{
		"input": CloseIssueInput{
			IssueID:     issue.ID,
			StateReason: reason,
		},
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	return gql.MutateNamed(context.Background(), "IssueClose", &mutation, variables)
}
