package reopen

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type ReopenOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string
	Comment     string
}

func NewCmdReopen(f *cmdutil.Factory, runF func(*ReopenOptions) error) *cobra.Command {
	opts := &ReopenOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "reopen {<number> | <url>}",
		Short: "Reopen issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return reopenRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "Add a reopening comment")

	return cmd
}

func reopenRun(opts *ReopenOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	issue, baseRepo, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, []string{"id", "number", "title", "state"})
	if err != nil {
		return err
	}

	if issue.State == "OPEN" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue %s#%d (%s) is already open\n", cs.Yellow("!"), ghrepo.FullName(baseRepo), issue.Number, issue.Title)
		return nil
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

	err = apiReopen(httpClient, baseRepo, issue)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Reopened issue %s#%d (%s)\n", cs.SuccessIconWithColor(cs.Green), ghrepo.FullName(baseRepo), issue.Number, issue.Title)

	return nil
}

func apiReopen(httpClient *http.Client, repo ghrepo.Interface, issue *api.Issue) error {
	if issue.IsPullRequest() {
		return api.PullRequestReopen(httpClient, repo, issue.ID)
	}

	var mutation struct {
		ReopenIssue struct {
			Issue struct {
				ID githubv4.ID
			}
		} `graphql:"reopenIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.ReopenIssueInput{
			IssueID: issue.ID,
		},
	}

	gql := api.NewClientFromHTTP(httpClient)
	return gql.Mutate(repo.RepoHost(), "IssueReopen", &mutation, variables)
}
